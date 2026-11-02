package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/user-service/internal/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, user domain.User) error {
	_, err := r.pool.Exec(ctx, `

		INSERT INTO users (id, email, password_hash, created_at, updated_at, firstname, lastname)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		user.ID, user.Email, user.PasswordHash, user.CreatedAt, user.UpdatedAt, user.Firstname, user.Lastname,
	)

	if isUniqueViolation(err) {
		return domain.ErrEmailAlreadyTaken
	}

	return err
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	row := r.pool.QueryRow(ctx, `

		SELECT id, email, password_hash, created_at, updated_at, firstname, lastname
		FROM users
		WHERE email = $1`, email)

	return scanUser(row)
}

func (r *UserRepository) List(ctx context.Context, offset, limit int) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, email, firstname, lastname
		FROM users
		ORDER BY created_at, id
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Email, &user.Firstname, &user.Lastname); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (r *UserRepository) Update(
	ctx context.Context,
	id domain.UserID,
	changes domain.UserUpdate,
	updatedAt time.Time,
) (domain.User, error) {
	row := r.pool.QueryRow(ctx, `

		UPDATE users
		SET email = COALESCE($2, email),
			password_hash = COALESCE($3, password_hash),
			firstname = COALESCE($4, firstname),
			lastname = COALESCE($5, lastname),
			updated_at = $6
		WHERE id = $1
		RETURNING id, email, password_hash, created_at, updated_at, firstname, lastname`,
		id, changes.Email, changes.PasswordHash, changes.Firstname, changes.Lastname, updatedAt,
	)
	user, err := scanUser(row)

	if isUniqueViolation(err) {
		return domain.User{}, domain.ErrEmailAlreadyTaken
	}

	return user, err
}

func (r *UserRepository) Delete(ctx context.Context, id domain.UserID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

type userRow interface {
	Scan(dest ...any) error
}

func scanUser(row userRow) (domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt, &user.Firstname, &user.Lastname)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
