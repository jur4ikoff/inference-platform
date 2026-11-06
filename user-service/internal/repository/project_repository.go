package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jur4ikoff/user-service/internal/domain"
	dao "github.com/jur4ikoff/user-service/internal/infrastructure/dao"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/utils/authjwt"
)

type ProjectRepository struct {
	pool Database
	q    *dao.Queries
}

func NewProjectRepository(pool Database) *ProjectRepository {
	return &ProjectRepository{pool: pool, q: dao.New(pool)}
}
func pgUUID() pgtype.UUID        { return pgtype.UUID{Bytes: uuid.New(), Valid: true} }
func pgTime() pgtype.Timestamptz { return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true} }
func projectID(id pgtype.UUID) domain.ProjectID {
	return domain.ProjectID(uuid.UUID(id.Bytes).String())
}
func rollbackProject(ctx context.Context, tx pgx.Tx) {
	c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(c)
}
func projectAudit(ctx context.Context, q *dao.Queries, actor, project pgtype.UUID, action string) error {
	return q.AuditProjectChange(ctx, dao.AuditProjectChangeParams{ID: pgUUID(), ActorID: actor, Action: action, ObjectID: project, CreatedAt: pgTime(), Details: []byte(`{}`)})
}
func (r *ProjectRepository) Create(ctx context.Context, actor domain.UserID, name, description string) (domain.Project, error) {
	a, err := userID(actor)
	if err != nil {
		return domain.Project{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Project{}, err
	}
	defer rollbackProject(ctx, tx)
	q := r.q.WithTx(tx)
	if _, err = q.LockUser(ctx, a); err != nil {
		return domain.Project{}, userError(err)
	}
	id, now := pgUUID(), pgTime()
	row, err := q.InsertProject(ctx, dao.InsertProjectParams{ID: id, Name: name, Description: description, CreatedAt: now})
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "23505" && pgerr.ConstraintName == "projects_name_key" {
			return domain.Project{}, domain.ErrProjectExists
		}
		return domain.Project{}, err
	}
	n, err := q.InsertTeamlead(ctx, dao.InsertTeamleadParams{ID: pgUUID(), UserID: a, ProjectID: id, CreatedAt: now})
	if err != nil {
		return domain.Project{}, err
	}
	if n != 1 {
		return domain.Project{}, errors.New("teamlead role is missing")
	}
	if err = projectAudit(ctx, q, a, id, "project.created"); err != nil {
		return domain.Project{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Project{}, err
	}
	return domain.Project{ID: projectID(row.ID), Name: row.Name, Description: row.Description}, nil
}
func (r *ProjectRepository) List(ctx context.Context, actor domain.UserID, name string, offset, limit int) ([]domain.Project, error) {
	a, err := userID(actor)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListVisibleProjects(ctx, dao.ListVisibleProjectsParams{ActorID: a, ProjectName: name, PageOffset: int32(offset), PageLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	result := make([]domain.Project, 0, len(rows))
	for _, p := range rows {
		result = append(result, domain.Project{ID: projectID(p.ID), Name: p.Name, Description: p.Description})
	}
	return result, nil
}

func (r *ProjectRepository) ProjectTokenGrant(ctx context.Context, actor domain.UserID, name string) (authjwt.ProjectGrant, error) {
	actorID, err := userID(actor)
	if err != nil {
		return authjwt.ProjectGrant{}, err
	}
	row, err := r.q.GetProjectTokenGrant(ctx, dao.GetProjectTokenGrantParams{ActorID: actorID, ProjectName: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return authjwt.ProjectGrant{}, domain.ErrProjectNotFound
	}
	if err != nil {
		return authjwt.ProjectGrant{}, err
	}
	return authjwt.ProjectGrant{Name: row.Name, Role: row.RoleName, Permissions: authjwt.Permissions{
		AllPermissions: row.AllPermissions, CanReadProject: row.CanReadProject, CanUpdateProject: row.CanUpdateProject, CanDeleteProject: row.CanDeleteProject, CanManageMembers: row.CanManageMembers,
		CanCreateModel: row.CanCreateModel, CanReadModel: row.CanReadModel, CanUpdateModel: row.CanUpdateModel, CanDeleteModel: row.CanDeleteModel, CanStartModel: row.CanStartModel, CanStopModel: row.CanStopModel,
	}}, nil
}

// access is checked under the project lock shared with membership mutations.
func lockedProject(ctx context.Context, q *dao.Queries, actor pgtype.UUID, name, action string) (dao.LockProjectRow, error) {
	p, err := q.LockProject(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, domain.ErrProjectNotFound
	}
	if err != nil {
		return p, err
	}
	a, err := q.GetProjectAccess(ctx, dao.GetProjectAccessParams{ProjectID: p.ID, ActorID: actor})
	if errors.Is(err, pgx.ErrNoRows) {
		return p, service.ErrInvalidToken
	}
	if err != nil {
		return p, err
	}
	if err = domain.PlatformRole(a.PlatformRole).Validate(); err != nil {
		return p, err
	}
	readable := a.PlatformRole == "admin" || a.PlatformRole == "analyst" || (a.IsMember && (a.AllPermissions || a.CanReadProject))
	if !readable {
		return p, domain.ErrProjectNotFound
	}
	allowed := false
	switch action {
	case "read":
		allowed = true
	case "update":
		allowed = a.PlatformRole == "admin" || (a.IsMember && (a.AllPermissions || a.CanUpdateProject))
	case "delete":
		allowed = a.PlatformRole == "admin" || (a.IsMember && (a.AllPermissions || a.CanDeleteProject))
	case "members-read":
		allowed = a.PlatformRole == "admin" || (a.IsMember && (a.AllPermissions || a.CanReadProject))
	case "members-write":
		allowed = a.PlatformRole == "admin" || (a.IsMember && (a.AllPermissions || a.CanManageMembers))
	}
	if !allowed {
		return p, service.ErrForbidden
	}
	return p, nil
}
func (r *ProjectRepository) Get(ctx context.Context, actor domain.UserID, name string) (domain.Project, error) {
	a, err := userID(actor)
	if err != nil {
		return domain.Project{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Project{}, err
	}
	defer rollbackProject(ctx, tx)
	p, err := lockedProject(ctx, r.q.WithTx(tx), a, name, "read")
	if err != nil {
		return domain.Project{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Project{}, err
	}
	return domain.Project{ID: projectID(p.ID), Name: p.Name, Description: p.Description}, nil
}
func (r *ProjectRepository) Update(ctx context.Context, actor domain.UserID, name, description string) (domain.Project, error) {
	a, err := userID(actor)
	if err != nil {
		return domain.Project{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Project{}, err
	}
	defer rollbackProject(ctx, tx)
	q := r.q.WithTx(tx)
	p, err := lockedProject(ctx, q, a, name, "update")
	if err != nil {
		return domain.Project{}, err
	}
	u, err := q.UpdateProjectDescription(ctx, dao.UpdateProjectDescriptionParams{ID: p.ID, Description: description, UpdatedAt: pgTime()})
	if err != nil {
		return domain.Project{}, err
	}
	if err = projectAudit(ctx, q, a, p.ID, "project.updated"); err != nil {
		return domain.Project{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Project{}, err
	}
	return domain.Project{ID: projectID(u.ID), Name: u.Name, Description: u.Description}, nil
}
func (r *ProjectRepository) Delete(ctx context.Context, actor domain.UserID, name string) error {
	a, err := userID(actor)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackProject(ctx, tx)
	q := r.q.WithTx(tx)
	p, err := lockedProject(ctx, q, a, name, "delete")
	if err != nil {
		return err
	}
	if err = q.SoftDeleteProject(ctx, dao.SoftDeleteProjectParams{ID: p.ID, DeletedAt: pgTime()}); err != nil {
		return err
	}
	if err = projectAudit(ctx, q, a, p.ID, "project.deleted"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r *ProjectRepository) Members(ctx context.Context, actor domain.UserID, name string, offset, limit int) ([]domain.Member, error) {
	a, err := userID(actor)
	if err != nil {
		return nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollbackProject(ctx, tx)
	q := r.q.WithTx(tx)
	p, err := lockedProject(ctx, q, a, name, "members-read")
	if err != nil {
		return nil, err
	}
	rows, err := q.ListProjectMembers(ctx, dao.ListProjectMembersParams{ProjectID: p.ID, Offset: int32(offset), Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	result := make([]domain.Member, 0, len(rows))
	for _, u := range rows {
		result = append(result, domain.Member{UserID: domain.UserID(uuid.UUID(u.ID.Bytes).String()), Email: u.Email, Firstname: u.Firstname, Lastname: u.Lastname, RoleID: domain.RoleID(u.RoleID), RoleName: u.RoleName})
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
func (r *ProjectRepository) ChangeMember(ctx context.Context, actor domain.UserID, name string, target domain.UserID, role domain.RoleID, remove bool) (bool, error) {
	a, err := userID(actor)
	if err != nil {
		return false, err
	}
	u, err := userID(target)
	if err != nil {
		return false, service.ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollbackProject(ctx, tx)
	q := r.q.WithTx(tx)
	// User deletion takes user locks before project locks; keep the same order.
	_, targetErr := q.LockUser(ctx, u)
	if targetErr != nil && !errors.Is(targetErr, pgx.ErrNoRows) {
		return false, targetErr
	}
	p, err := lockedProject(ctx, q, a, name, "members-write")
	if err != nil {
		return false, err
	}
	if errors.Is(targetErr, pgx.ErrNoRows) {
		return false, domain.ErrUserNotFound
	}
	old, err := q.GetMemberRoleKey(ctx, dao.GetMemberRoleKeyParams{ProjectID: p.ID, UserID: u})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	exists := err == nil
	if remove && !exists {
		return false, domain.ErrMemberNotFound
	}
	newRole := ""
	if !remove {
		newRole, err = q.GetRoleSystemKey(ctx, int32(role))
		if errors.Is(err, pgx.ErrNoRows) {
			return false, domain.ErrRoleNotFound
		}
		if err != nil {
			return false, err
		}
	}
	if exists && old.SystemKey == "teamlead" && (remove || newRole != "teamlead") {
		n, e := q.CountTeamleads(ctx, p.ID)
		if e != nil {
			return false, e
		}
		if n <= 1 {
			return false, domain.ErrLastTeamlead
		}
	}
	if remove {
		_, err = q.DeleteMember(ctx, dao.DeleteMemberParams{ProjectID: p.ID, UserID: u})
	} else {
		err = q.UpsertMember(ctx, dao.UpsertMemberParams{ID: pgUUID(), UserID: u, ProjectID: p.ID, RoleID: int32(role), CreatedAt: pgTime()})
	}
	if err != nil {
		return false, err
	}
	action := "member.assigned"
	if remove {
		action = "member.removed"
	}
	fields := map[string]any{}
	if exists {
		fields["previous_role_id"] = old.RoleID
	}
	if !remove {
		fields["role_id"] = role
	}
	details, err := json.Marshal(fields)
	if err != nil {
		return false, err
	}
	if err = q.AuditProjectChange(ctx, dao.AuditProjectChangeParams{ID: pgUUID(), ActorID: a, Action: action, ObjectID: p.ID, SubjectID: u, Details: details, CreatedAt: pgTime()}); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return !exists && !remove, nil
}
