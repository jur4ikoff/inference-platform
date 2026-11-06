package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jur4ikoff/user-service/internal/domain"
	dao "github.com/jur4ikoff/user-service/internal/infrastructure/dao"
	"github.com/jur4ikoff/user-service/internal/service"
)

type RoleRepository struct {
	pool    Database
	queries *dao.Queries
}

func NewRoleRepository(pool Database) *RoleRepository {
	return &RoleRepository{pool: pool, queries: dao.New(pool)}
}

func boolValue(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}
func roleFrom(id int32, name, systemKey string, all, read, update, del, members, createModel, readModel, updateModel, deleteModel, start, stop bool) domain.Role {
	return domain.Role{ID: domain.RoleID(id), Name: name, SystemKey: systemKey, AllPermissions: all, CanReadProject: read, CanUpdateProject: update, CanDeleteProject: del, CanManageMembers: members, CanCreateModel: createModel, CanReadModel: readModel, CanUpdateModel: updateModel, CanDeleteModel: deleteModel, CanStartModel: start, CanStopModel: stop}
}
func roleFromGet(r dao.GetRoleRow) domain.Role {
	return roleFrom(r.ID, r.Name, r.SystemKey.String, r.AllPermissions, r.CanReadProject, r.CanUpdateProject, r.CanDeleteProject, r.CanManageMembers, r.CanCreateModel, r.CanReadModel, r.CanUpdateModel, r.CanDeleteModel, r.CanStartModel, r.CanStopModel)
}
func roleFromCreate(r dao.CreateRoleRow) domain.Role {
	return roleFrom(r.ID, r.Name, r.SystemKey.String, r.AllPermissions, r.CanReadProject, r.CanUpdateProject, r.CanDeleteProject, r.CanManageMembers, r.CanCreateModel, r.CanReadModel, r.CanUpdateModel, r.CanDeleteModel, r.CanStartModel, r.CanStopModel)
}
func roleFromUpdate(r dao.UpdateRoleRow) domain.Role {
	return roleFrom(r.ID, r.Name, r.SystemKey.String, r.AllPermissions, r.CanReadProject, r.CanUpdateProject, r.CanDeleteProject, r.CanManageMembers, r.CanCreateModel, r.CanReadModel, r.CanUpdateModel, r.CanDeleteModel, r.CanStartModel, r.CanStopModel)
}
func roleFromList(r dao.ListRolesRow) domain.Role {
	return roleFrom(r.ID, r.Name, r.SystemKey.String, r.AllPermissions, r.CanReadProject, r.CanUpdateProject, r.CanDeleteProject, r.CanManageMembers, r.CanCreateModel, r.CanReadModel, r.CanUpdateModel, r.CanDeleteModel, r.CanStartModel, r.CanStopModel)
}

func roleSnapshot(role domain.Role) map[string]any {
	return map[string]any{
		"name":               role.Name,
		"all_permissions":    role.AllPermissions,
		"can_read_project":   role.CanReadProject,
		"can_update_project": role.CanUpdateProject,
		"can_delete_project": role.CanDeleteProject,
		"can_manage_members": role.CanManageMembers,
		"can_create_model":   role.CanCreateModel,
		"can_read_model":     role.CanReadModel,
		"can_update_model":   role.CanUpdateModel,
		"can_delete_model":   role.CanDeleteModel,
		"can_start_model":    role.CanStartModel,
		"can_stop_model":     role.CanStopModel,
	}
}

func roleAudit(ctx context.Context, q *dao.Queries, actor pgtype.UUID, action string, id int32, details any) error {
	fields, ok := details.(map[string]any)
	if !ok {
		fields = map[string]any{}
	}
	fields["role_id"] = id
	b, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	objectID := pgtype.UUID{Bytes: uuid.NewSHA1(uuid.NameSpaceOID, []byte("role:"+strconv.Itoa(int(id)))), Valid: true}
	return q.AuditRoleChange(ctx, dao.AuditRoleChangeParams{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, ActorID: actor, Action: action, ObjectID: objectID, CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, Details: b})
}
func requireAdmin(ctx context.Context, q *dao.Queries, actor pgtype.UUID) error {
	if err := q.LockAdminRegistry(ctx); err != nil {
		return err
	}
	role, err := q.LockUser(ctx, actor)
	if errors.Is(err, pgx.ErrNoRows) {
		return service.ErrInvalidToken
	}
	if err != nil {
		return err
	}
	if role != "admin" {
		return service.ErrForbidden
	}
	return nil
}
func roleWriteError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) {
		if p.Code == "23505" && p.ConstraintName == "roles_name_key" {
			return domain.ErrRoleAlreadyExists
		}
		if p.Code == "23503" {
			return domain.ErrRoleAssigned
		}
	}
	return err
}
func (r *RoleRepository) transaction(ctx context.Context, actor domain.UserID, fn func(*dao.Queries, pgtype.UUID) error) error {
	a, err := userID(actor)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(c)
	}()
	q := r.queries.WithTx(tx)
	if err = requireAdmin(ctx, q, a); err != nil {
		return err
	}
	if err = fn(q, a); err != nil {
		return roleWriteError(err)
	}
	return tx.Commit(ctx)
}
func (r *RoleRepository) Create(ctx context.Context, actor domain.UserID, role domain.Role) (domain.Role, error) {
	var out domain.Role
	err := r.transaction(ctx, actor, func(q *dao.Queries, a pgtype.UUID) error {
		row, e := q.CreateRole(ctx, dao.CreateRoleParams{Name: role.Name, CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, AllPermissions: role.AllPermissions, CanReadProject: role.CanReadProject, CanUpdateProject: role.CanUpdateProject, CanDeleteProject: role.CanDeleteProject, CanManageMembers: role.CanManageMembers, CanCreateModel: role.CanCreateModel, CanReadModel: role.CanReadModel, CanUpdateModel: role.CanUpdateModel, CanDeleteModel: role.CanDeleteModel, CanStartModel: role.CanStartModel, CanStopModel: role.CanStopModel})
		if e != nil {
			return e
		}
		out = roleFromCreate(row)
		return roleAudit(ctx, q, a, "role.created", row.ID, map[string]any{"after": roleSnapshot(out)})
	})
	return out, err
}
func (r *RoleRepository) Get(ctx context.Context, id domain.RoleID) (domain.Role, error) {
	row, err := r.queries.GetRole(ctx, int32(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Role{}, domain.ErrRoleNotFound
	}
	if err != nil {
		return domain.Role{}, err
	}
	return roleFromGet(row), nil
}
func (r *RoleRepository) List(ctx context.Context, name string, offset, limit int) ([]domain.Role, error) {
	rows, err := r.queries.ListRoles(ctx, dao.ListRolesParams{Name: name, PageLimit: int32(limit), PageOffset: int32(offset)})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Role, 0, len(rows))
	for _, row := range rows {
		out = append(out, roleFromList(row))
	}
	return out, nil
}
func (r *RoleRepository) Update(ctx context.Context, actor domain.UserID, id domain.RoleID, update domain.RoleUpdate) (domain.Role, error) {
	var out domain.Role
	err := r.transaction(ctx, actor, func(q *dao.Queries, a pgtype.UUID) error {
		locked, e := q.LockRole(ctx, int32(id))
		if errors.Is(e, pgx.ErrNoRows) {
			return domain.ErrRoleNotFound
		}
		if e != nil {
			return e
		}
		if locked.SystemKey.Valid && locked.SystemKey.String == "teamlead" && update.AllPermissions != nil && !*update.AllPermissions {
			return domain.ErrSystemRole
		}
		before := roleFrom(locked.ID, locked.Name, locked.SystemKey.String, locked.AllPermissions, locked.CanReadProject, locked.CanUpdateProject, locked.CanDeleteProject, locked.CanManageMembers, locked.CanCreateModel, locked.CanReadModel, locked.CanUpdateModel, locked.CanDeleteModel, locked.CanStartModel, locked.CanStopModel)
		row, e := q.UpdateRole(ctx, dao.UpdateRoleParams{ID: int32(id), UpdatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, Name: nullable(update.Name), AllPermissions: boolValue(update.AllPermissions), CanReadProject: boolValue(update.CanReadProject), CanUpdateProject: boolValue(update.CanUpdateProject), CanDeleteProject: boolValue(update.CanDeleteProject), CanManageMembers: boolValue(update.CanManageMembers), CanCreateModel: boolValue(update.CanCreateModel), CanReadModel: boolValue(update.CanReadModel), CanUpdateModel: boolValue(update.CanUpdateModel), CanDeleteModel: boolValue(update.CanDeleteModel), CanStartModel: boolValue(update.CanStartModel), CanStopModel: boolValue(update.CanStopModel)})
		if e != nil {
			return e
		}
		out = roleFromUpdate(row)
		return roleAudit(ctx, q, a, "role.updated", row.ID, map[string]any{"before": roleSnapshot(before), "after": roleSnapshot(out)})
	})
	return out, err
}
func (r *RoleRepository) Delete(ctx context.Context, actor domain.UserID, id domain.RoleID) error {
	return r.transaction(ctx, actor, func(q *dao.Queries, a pgtype.UUID) error {
		role, e := q.LockRole(ctx, int32(id))
		if errors.Is(e, pgx.ErrNoRows) {
			return domain.ErrRoleNotFound
		}
		if e != nil {
			return e
		}
		if role.SystemKey.Valid && role.SystemKey.String == "teamlead" {
			return domain.ErrSystemRole
		}
		n, e := q.DeleteRole(ctx, int32(id))
		if e != nil {
			return e
		}
		if n == 0 {
			return domain.ErrRoleNotFound
		}
		before := roleFrom(role.ID, role.Name, role.SystemKey.String, role.AllPermissions, role.CanReadProject, role.CanUpdateProject, role.CanDeleteProject, role.CanManageMembers, role.CanCreateModel, role.CanReadModel, role.CanUpdateModel, role.CanDeleteModel, role.CanStartModel, role.CanStopModel)
		return roleAudit(ctx, q, a, "role.deleted", int32(id), map[string]any{"before": roleSnapshot(before)})
	})
}
