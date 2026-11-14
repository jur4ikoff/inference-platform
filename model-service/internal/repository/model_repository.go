package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jur4ikoff/model-service/internal/domain"
	modeldao "github.com/jur4ikoff/model-service/internal/infrastructure/dao"
)

type ModelRepository struct {
	queries     *modeldao.Queries
	readQueries *modeldao.Queries
}

type replicaReadKey struct{}

func WithReplicaRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, replicaReadKey{}, true)
}

func NewModelRepository(pool modeldao.DBTX, readers ...modeldao.DBTX) *ModelRepository {
	r := &ModelRepository{queries: modeldao.New(pool)}
	if len(readers) > 0 && readers[0] != nil {
		r.readQueries = modeldao.New(readers[0])
	}
	return r
}

func (r *ModelRepository) reader(ctx context.Context) *modeldao.Queries {
	if useReplica, _ := ctx.Value(replicaReadKey{}).(bool); useReplica && r.readQueries != nil {
		return r.readQueries
	}
	return r.queries
}

func (r *ModelRepository) Create(ctx context.Context, model domain.Model) (domain.Model, error) {
	spec, err := model.Spec.CanonicalJSON()
	if err != nil {
		return domain.Model{}, fmt.Errorf("encode model spec: %w", err)
	}
	row, err := r.queries.CreateModel(ctx, modeldao.CreateModelParams{
		ID:          uuidValue(model.ID),
		CreatedAt:   timestampValue(model.CreatedAt),
		UpdatedAt:   timestampValue(model.UpdatedAt),
		Name:        model.Name,
		Description: model.Description,
		Username:    model.Username,
		Project:     model.Project,
		Spec:        spec,
	})
	if err != nil {
		return domain.Model{}, modelWriteError(err)
	}
	saved, err := modelFromDAO(row)
	if err != nil {
		return domain.Model{}, fmt.Errorf("decode created model: %w", err)
	}
	return saved, nil
}

func (r *ModelRepository) Get(ctx context.Context, project, modelID string) (domain.Model, error) {
	row, err := r.reader(ctx).GetModel(ctx, modeldao.GetModelParams{Project: project, ID: uuidValue(modelID)})
	return modelResult(row, err)
}

func (r *ModelRepository) List(ctx context.Context, project string, offset, limit int) ([]domain.Model, error) {
	rows, err := r.reader(ctx).ListModels(ctx, modeldao.ListModelsParams{
		Project:    project,
		PageLimit:  int64(limit),
		PageOffset: int64(offset),
	})
	if err != nil {
		return nil, err
	}
	models := make([]domain.Model, 0, len(rows))
	for _, row := range rows {
		model, err := modelFromDAO(row)
		if err != nil {
			return nil, fmt.Errorf("decode listed model: %w", err)
		}
		models = append(models, model)
	}
	return models, nil
}

func (r *ModelRepository) Update(ctx context.Context, project, modelID string, name, description *string, spec *domain.ModelSpec, updatedAt time.Time) (domain.Model, error) {
	var encodedSpec []byte
	if spec != nil {
		var err error
		encodedSpec, err = spec.CanonicalJSON()
		if err != nil {
			return domain.Model{}, fmt.Errorf("encode model spec: %w", err)
		}
	}
	row, err := r.queries.UpdateModel(ctx, modeldao.UpdateModelParams{
		Name:        optionalTextValue(name),
		Description: optionalTextValue(description),
		Spec:        encodedSpec,
		UpdatedAt:   timestampValue(updatedAt),
		Project:     project,
		ID:          uuidValue(modelID),
	})
	return modelResult(row, modelWriteError(err))
}

func (r *ModelRepository) Delete(ctx context.Context, project, modelID string) error {
	rowsAffected, err := r.queries.DeleteModel(ctx, modeldao.DeleteModelParams{Project: project, ID: uuidValue(modelID)})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrModelNotFound
	}
	return nil
}

func modelResult(row modeldao.Model, err error) (domain.Model, error) {
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Model{}, domain.ErrModelNotFound
		}
		return domain.Model{}, err
	}
	return modelFromDAO(row)
}

func modelWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "models_project_name_key" {
		return domain.ErrModelAlreadyExists
	}
	return err
}

func modelFromDAO(row modeldao.Model) (domain.Model, error) {
	if !row.ID.Valid || !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return domain.Model{}, errors.New("model row contains a null required field")
	}
	spec, _, err := domain.ParseModelSpec(row.Spec)
	if err != nil {
		return domain.Model{}, fmt.Errorf("decode model spec: %w", err)
	}
	model := domain.Model{
		ID:          uuid.UUID(row.ID.Bytes).String(),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
		Name:        row.Name,
		Description: row.Description,
		Username:    row.Username,
		Project:     row.Project,
		Spec:        spec,
	}
	return model, nil
}

func uuidValue(value string) pgtype.UUID {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

func timestampValue(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func optionalTextValue(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
