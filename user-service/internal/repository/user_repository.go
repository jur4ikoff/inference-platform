package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jur4ikoff/user-service/internal/domain"
	postgres "github.com/jur4ikoff/user-service/internal/infrastructure/dao"
	"github.com/jur4ikoff/user-service/internal/service"
)

// Database supports queries and transactions used by the repositories.
type Database interface {
	postgres.DBTX
	Begin(context.Context) (pgx.Tx, error)
}

type UserRepository struct {
	queries *postgres.Queries
	pool    Database
}

func NewUserRepository(pool Database) *UserRepository {
	return &UserRepository{queries: postgres.New(pool), pool: pool}
}
func userID(id domain.UserID) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid user id: %w", err)
	}
	if parsed == uuid.Nil {
		return pgtype.UUID{}, errors.New("user id must not be zero")
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}
func nullable(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}
func mapUser(u postgres.User) domain.User {
	return domain.User{ID: domain.UserID(uuid.UUID(u.ID.Bytes).String()), Email: u.Email, Login: u.Login, PasswordHash: u.PasswordHash, PlatformRole: domain.PlatformRole(u.PlatformRole), Firstname: u.Firstname, Lastname: u.Lastname, CreatedAt: u.CreatedAt.Time, UpdatedAt: u.UpdatedAt.Time}
}
func userError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.Join(domain.ErrUserNotFound, err)
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" && p.ConstraintName == "users_email_lower_unique_idx" {
		return errors.Join(domain.ErrEmailAlreadyTaken, err)
	}
	if errors.As(err, &p) && p.Code == "23505" && p.ConstraintName == "users_login_lower_unique_idx" {
		return errors.Join(domain.ErrLoginAlreadyTaken, err)
	}
	return err
}
func auditUser(ctx context.Context, q *postgres.Queries, id pgtype.UUID, action string, details ...any) error {
	eventID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	actor := pgtype.UUID{}
	if p, ok := service.PrincipalFromContext(ctx); ok {
		actor, err = userID(p.UserID)
		if err != nil {
			return err
		}
	}
	payload := []byte("{}")
	if len(details) > 0 && details[0] != nil {
		payload, err = json.Marshal(details[0])
		if err != nil {
			return err
		}
	}
	return q.AuditUserChange(ctx, postgres.AuditUserChangeParams{ID: pgtype.UUID{Bytes: eventID, Valid: true}, ActorID: actor, Action: action, ObjectID: id, CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, Details: payload})
}
func (r *UserRepository) Create(ctx context.Context, u domain.User) error {
	_, err := r.CreateSaved(ctx, u)
	return err
}
func (r *UserRepository) CreateSaved(ctx context.Context, u domain.User) (domain.User, error) {
	id, err := userID(u.ID)
	if err != nil {
		return domain.User{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	q := r.queries.WithTx(tx)
	row, err := q.CreateUser(ctx, postgres.CreateUserParams{ID: id, Email: u.Email, Login: u.Login, PasswordHash: u.PasswordHash, Firstname: u.Firstname, Lastname: u.Lastname, CreatedAt: pgtype.Timestamptz{Time: u.CreatedAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: u.UpdatedAt, Valid: true}})
	if err != nil {
		return domain.User{}, userError(err)
	}
	if err = auditUser(ctx, q, id, "user.created"); err != nil {
		return domain.User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	return mapUser(row), nil
}

func (r *UserRepository) BootstrapAdmin(ctx context.Context, user domain.User) (domain.User, bool, error) {
	id, err := userID(user.ID)
	if err != nil {
		return domain.User{}, false, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, false, err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	q := r.queries.WithTx(tx)
	if err = q.LockAdminRegistry(ctx); err != nil {
		return domain.User{}, false, err
	}
	existing, findErr := q.FindUserByEmailForUpdate(ctx, user.Email)
	if findErr == nil {
		saved := mapUser(existing)
		if saved.PlatformRole != domain.PlatformRoleAdmin {
			return domain.User{}, false, domain.ErrEmailAlreadyTaken
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.User{}, false, err
		}
		return saved, true, nil
	}
	if !errors.Is(findErr, pgx.ErrNoRows) {
		return domain.User{}, false, findErr
	}
	n, err := q.CountAdmins(ctx)
	if err != nil {
		return domain.User{}, false, err
	}
	if n != 0 {
		return domain.User{}, false, domain.ErrBootstrapAdminExists
	}
	row, err := q.CreateAdminUser(ctx, postgres.CreateAdminUserParams{ID: id, Email: user.Email, Login: user.Login, PasswordHash: user.PasswordHash, Firstname: user.Firstname, Lastname: user.Lastname, CreatedAt: pgtype.Timestamptz{Time: user.CreatedAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: user.UpdatedAt, Valid: true}})
	if err != nil {
		return domain.User{}, false, userError(err)
	}
	if err = auditUser(ctx, q, id, "user.bootstrap_admin_created"); err != nil {
		return domain.User{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, false, err
	}
	return mapUser(row), false, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	u, err := r.queries.FindUserByEmail(ctx, email)
	if err != nil {
		return domain.User{}, userError(err)
	}
	return mapUser(u), nil
}
func (r *UserRepository) FindByLogin(ctx context.Context, login string) (domain.User, error) {
	u, err := r.queries.FindUserByLogin(ctx, login)
	if err != nil {
		return domain.User{}, userError(err)
	}
	return mapUser(u), nil
}
func (r *UserRepository) List(ctx context.Context, offset, limit int) ([]domain.User, error) {
	if offset < 0 || limit < 0 || offset > math.MaxInt32 || limit > math.MaxInt32 {
		return nil, fmt.Errorf("invalid pagination")
	}
	rows, err := r.queries.ListUsers(ctx, postgres.ListUsersParams{Offset: int32(offset), Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	result := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.User{ID: domain.UserID(uuid.UUID(row.ID.Bytes).String()), Email: row.Email, Firstname: row.Firstname, Lastname: row.Lastname})
	}
	return result, nil
}
func (r *UserRepository) Update(ctx context.Context, id domain.UserID, c domain.UserUpdate, updated time.Time) (domain.User, error) {
	uid, err := userID(id)
	if err != nil {
		return domain.User{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	q := r.queries.WithTx(tx)
	u, err := q.UpdateUser(ctx, postgres.UpdateUserParams{ID: uid, Email: nullable(c.Email), PasswordHash: nullable(c.PasswordHash), Firstname: nullable(c.Firstname), Lastname: nullable(c.Lastname), UpdatedAt: pgtype.Timestamptz{Time: updated, Valid: true}})
	if err != nil {
		return domain.User{}, userError(err)
	}
	if err = auditUser(ctx, q, uid, "user.updated"); err != nil {
		return domain.User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	return mapUser(u), nil
}

func (r *UserRepository) Delete(ctx context.Context, id domain.UserID) error {
	uid, err := userID(id)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	q := r.queries.WithTx(tx)
	if err = q.LockAdminRegistry(ctx); err != nil {
		return err
	}
	role, err := q.LockUser(ctx, uid)
	if err != nil {
		return userError(err)
	}
	if role == "admin" {
		n, e := q.CountOtherAdmins(ctx, uid)
		if e != nil {
			return e
		}
		if n == 0 {
			return domain.ErrLastAdministrator
		}
	}
	if _, err = q.LockUserProjects(ctx, uid); err != nil {
		return err
	}
	n, err := q.CountOrphanedProjects(ctx, uid)
	if err != nil {
		return err
	}
	if n > 0 {
		return domain.ErrLastTeamlead
	}
	count, err := q.DeleteUser(ctx, uid)
	if err != nil {
		return userError(err)
	}
	if count == 0 {
		return domain.ErrUserNotFound
	}
	if err = auditUser(ctx, q, uid, "user.deleted"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *UserRepository) GetAuthState(ctx context.Context, actor domain.UserID) (domain.AuthState, error) {
	id, err := userID(actor)
	if err != nil {
		return domain.AuthState{}, err
	}
	row, err := r.queries.GetAuthState(ctx, id)
	if err != nil {
		return domain.AuthState{}, userError(err)
	}
	role := domain.PlatformRole(row.PlatformRole)
	if err = role.Validate(); err != nil {
		return domain.AuthState{}, err
	}
	return domain.AuthState{UserID: domain.UserID(uuid.UUID(row.ID.Bytes).String()), PlatformRole: role, AuthVersion: row.AuthVersion}, nil
}

// UpdatePasswordHashIfCurrent upgrades a legacy password only when the
// credential verified during login is still the stored credential. It closes
// the race where a password changes between Compare and token issuance.
func (r *UserRepository) UpdatePasswordHashIfCurrent(ctx context.Context, id domain.UserID, previousHash, newHash string, updatedAt time.Time) (domain.AuthState, bool, error) {
	uid, err := userID(id)
	if err != nil {
		return domain.AuthState{}, false, err
	}
	row, err := r.queries.UpdatePasswordHashIfCurrent(ctx, postgres.UpdatePasswordHashIfCurrentParams{
		ID: uid, PreviousHash: previousHash, NewHash: newHash,
		UpdatedAt: pgtype.Timestamptz{Time: updatedAt.UTC(), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AuthState{}, false, nil
	}
	if err != nil {
		return domain.AuthState{}, false, err
	}
	return domain.AuthState{UserID: id, PlatformRole: domain.PlatformRole(row.PlatformRole), AuthVersion: row.AuthVersion}, true, nil
}
func (r *UserRepository) ActorRole(ctx context.Context, actor domain.UserID) (string, error) {
	state, err := r.GetAuthState(ctx, actor)
	return string(state.PlatformRole), err
}

// SetPlatformRole serializes global-role changes with account deletion and
// verifies the initiator from the database inside the transaction.
func (r *UserRepository) SetPlatformRole(ctx context.Context, actor, target domain.UserID, role domain.PlatformRole) (domain.User, error) {
	actorID, err := userID(actor)
	if err != nil {
		return domain.User{}, err
	}
	targetID, err := userID(target)
	if err != nil {
		return domain.User{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	q := r.queries.WithTx(tx)
	if err = q.LockAdminRegistry(ctx); err != nil {
		return domain.User{}, err
	}
	actorRole, err := q.LockUser(ctx, actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, service.ErrInvalidToken
	}
	if err != nil {
		return domain.User{}, err
	}
	if actorRole != string(domain.PlatformRoleAdmin) {
		return domain.User{}, service.ErrForbidden
	}
	targetRole, err := q.LockUser(ctx, targetID)
	if err != nil {
		return domain.User{}, userError(err)
	}
	if targetRole == string(role) {
		return r.getLockedUser(ctx, q, targetID)
	}
	if targetRole == string(domain.PlatformRoleAdmin) && role != domain.PlatformRoleAdmin {
		n, countErr := q.CountOtherAdmins(ctx, targetID)
		if countErr != nil {
			return domain.User{}, countErr
		}
		if n == 0 {
			return domain.User{}, domain.ErrLastAdministrator
		}
	}
	updated, err := q.UpdatePlatformRole(ctx, postgres.UpdatePlatformRoleParams{ID: targetID, PlatformRole: string(role), UpdatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
	if err != nil {
		return domain.User{}, userError(err)
	}
	if err = auditUser(ctx, q, targetID, "user.platform_role_updated", map[string]string{
		"old_platform_role": targetRole,
		"new_platform_role": string(role),
	}); err != nil {
		return domain.User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	return mapUser(updated), nil
}

func (r *UserRepository) getLockedUser(ctx context.Context, q *postgres.Queries, id pgtype.UUID) (domain.User, error) {
	row, err := q.GetUserByID(ctx, id)
	if err != nil {
		return domain.User{}, userError(err)
	}
	return mapUser(row), nil
}

func (r *UserRepository) GetVisible(ctx context.Context, actor, target domain.UserID) (domain.User, error) {
	a, err := userID(actor)
	if err != nil {
		return domain.User{}, err
	}
	t, err := userID(target)
	if err != nil {
		return domain.User{}, err
	}
	row, err := r.queries.GetVisibleUser(ctx, postgres.GetVisibleUserParams{ActorID: a, TargetID: t})
	if err != nil {
		return domain.User{}, userError(err)
	}
	return domain.User{ID: domain.UserID(uuid.UUID(row.ID.Bytes).String()), Email: row.Email, Firstname: row.Firstname, Lastname: row.Lastname, PlatformRole: domain.PlatformRole(row.VisiblePlatformRole)}, nil
}
func (r *UserRepository) ListVisible(ctx context.Context, actor domain.UserID, offset, limit int) ([]domain.User, error) {
	if offset < 0 || limit < 0 || offset > math.MaxInt32 || limit > math.MaxInt32 {
		return nil, fmt.Errorf("invalid pagination")
	}
	a, err := userID(actor)
	if err != nil {
		return nil, err
	}
	rows, err := r.queries.ListVisibleUsers(ctx, postgres.ListVisibleUsersParams{ActorID: a, PageOffset: int32(offset), PageLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.User{ID: domain.UserID(uuid.UUID(row.ID.Bytes).String()), Email: row.Email, Firstname: row.Firstname, Lastname: row.Lastname, PlatformRole: domain.PlatformRole(row.VisiblePlatformRole)})
	}
	return out, nil
}
