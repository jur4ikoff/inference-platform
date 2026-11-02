package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/model-service/internal/domain"
)

type ModelRepository struct {
	pool *pgxpool.Pool
}

func NewModelRepository(pool *pgxpool.Pool) *ModelRepository {
	return &ModelRepository{pool: pool}
}

func (r *ModelRepository) Create(ctx context.Context, model domain.Model) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO models (id, created_at, updated_at, name, description, user_id, project, spec)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		model.ID, model.CreatedAt, model.UpdatedAt, model.Name, model.Description, model.UserID, model.Project, model.Spec,
	)
	return modelWriteError(err)
}

func (r *ModelRepository) Get(ctx context.Context, project, modelID string) (domain.Model, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, created_at, updated_at, name, description, user_id, project, spec
		FROM models WHERE project = $1 AND id = $2`, project, modelID)
	return scanModel(row)
}

func (r *ModelRepository) List(ctx context.Context, project string, offset, limit int) ([]domain.Model, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, created_at, updated_at, name, description, user_id, project, spec
		FROM models
		WHERE project = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`, project, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := make([]domain.Model, 0)
	for rows.Next() {
		model, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, rows.Err()
}

func (r *ModelRepository) Update(ctx context.Context, project, modelID string, name, description *string, spec json.RawMessage, updatedAt time.Time) (domain.Model, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE models
		SET name = COALESCE($3, name),
			description = COALESCE($4, description),
			spec = COALESCE($5, spec),
			updated_at = $6
		WHERE project = $1 AND id = $2
		RETURNING id, created_at, updated_at, name, description, user_id, project, spec`,
		project, modelID, name, description, spec, updatedAt,
	)
	model, err := scanModel(row)
	return model, modelWriteError(err)
}

func (r *ModelRepository) Delete(ctx context.Context, project, modelID string) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM models WHERE project = $1 AND id = $2`, project, modelID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return domain.ErrModelNotFound
	}
	return nil
}

func modelWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "models_project_name_key" {
		return domain.ErrModelAlreadyExists
	}
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanModel(row rowScanner) (domain.Model, error) {
	var model domain.Model
	if err := row.Scan(&model.ID, &model.CreatedAt, &model.UpdatedAt, &model.Name, &model.Description, &model.UserID, &model.Project, &model.Spec); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Model{}, domain.ErrModelNotFound
		}
		return domain.Model{}, err
	}
	model.Spec = json.RawMessage(strings.TrimSpace(string(model.Spec)))
	return model, nil
}
