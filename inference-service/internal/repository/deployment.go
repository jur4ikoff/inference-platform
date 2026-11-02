package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jur4ikoff/inference-service/internal/domain"
	postgres "github.com/jur4ikoff/inference-service/internal/infrastructure/dao"
)

type DeploymentRepository struct {
	queries *postgres.Queries
}

func NewDeploymentRepository(db postgres.DBTX) *DeploymentRepository {
	return &DeploymentRepository{queries: postgres.New(db)}
}

func (r *DeploymentRepository) Check(ctx context.Context) error {
	if _, err := r.queries.CheckConnection(ctx); err != nil {
		return fmt.Errorf("check PostgreSQL connection: %w", err)
	}
	return nil
}

func (r *DeploymentRepository) ActiveDeploymentExists(ctx context.Context, project string, modelID uuid.UUID) (bool, error) {
	exists, err := r.queries.ActiveDeploymentExists(ctx, postgres.ActiveDeploymentExistsParams{
		Project: project,
		ModelID: pgtype.UUID{Bytes: modelID, Valid: true},
		Statuses: []string{
			string(domain.StatusPending), string(domain.StatusStarting),
			string(domain.StatusStarted), string(domain.StatusRunning), string(domain.StatusStopping),
		},
	})
	if err != nil {
		return false, fmt.Errorf("check deployment existence: %w", err)
	}
	return exists, nil
}

func (r *DeploymentRepository) CreateDeployment(ctx context.Context, deployment domain.Deployment) (domain.Deployment, error) {
	if err := deployment.Status.Validate(); err != nil {
		return domain.Deployment{}, fmt.Errorf("create deployment: %w", err)
	}
	spec, err := json.Marshal(deployment.Spec)
	if err != nil {
		return domain.Deployment{}, fmt.Errorf("encode deployment spec: %w", err)
	}
	row, err := r.queries.CreateDeployment(ctx, postgres.CreateDeploymentParams{
		ID:             pgtype.UUID{Bytes: deployment.ID, Valid: true},
		ContainerID:    deployment.ContainerID,
		ModelID:        pgtype.UUID{Bytes: deployment.ModelID, Valid: true},
		Project:        deployment.Project,
		Username:       deployment.Username,
		Name:           deployment.Name,
		DeploymentName: deployment.DeploymentName,
		Status:         string(deployment.Status),
		Spec:           spec,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && (pgErr.ConstraintName == "deployments_project_model_id_key" || pgErr.ConstraintName == "deployments_project_deployment_name_key") {
			err = errors.Join(domain.ErrDeploymentAlreadyExists, err)
		}
		return domain.Deployment{}, fmt.Errorf("insert deployment: %w", err)
	}
	return deploymentFromRow(row)
}

func (r *DeploymentRepository) GetDeploymentByID(ctx context.Context, id uuid.UUID) (domain.Deployment, error) {
	row, err := r.queries.GetDeploymentByID(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Deployment{}, fmt.Errorf("%w: %w", domain.ErrDeploymentNotFound, err)
	}
	if err != nil {
		return domain.Deployment{}, fmt.Errorf("get deployment by ID: %w", err)
	}
	return deploymentFromRow(row)
}

func (r *DeploymentRepository) GetDeploymentByName(ctx context.Context, name string) (domain.Deployment, error) {
	row, err := r.queries.GetDeploymentByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Deployment{}, fmt.Errorf("%w: %w", domain.ErrDeploymentNotFound, err)
	}
	if err != nil {
		return domain.Deployment{}, fmt.Errorf("get deployment by name: %w", err)
	}
	return deploymentFromRow(row)
}

func (r *DeploymentRepository) ListDeployments(ctx context.Context, offset, limit int) ([]domain.Deployment, error) {
	if err := validatePageBounds(offset, limit); err != nil {
		return nil, err
	}
	rows, err := r.queries.ListDeployments(ctx, postgres.ListDeploymentsParams{
		PageOffset: int32(offset), PageLimit: int32(limit),
		Statuses: []string{
			string(domain.StatusPending), string(domain.StatusStarting),
			string(domain.StatusStarted), string(domain.StatusRunning),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	return deploymentsFromRows(rows)
}

func (r *DeploymentRepository) ListAllDeployments(ctx context.Context, offset, limit int) ([]domain.Deployment, error) {
	if err := validatePageBounds(offset, limit); err != nil {
		return nil, err
	}
	rows, err := r.queries.ListAllDeployments(ctx, postgres.ListAllDeploymentsParams{PageOffset: int32(offset), PageLimit: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("list all deployments: %w", err)
	}
	return deploymentsFromRows(rows)
}

func validatePageBounds(offset, limit int) error {
	if offset < 0 || offset > math.MaxInt32 || limit < 1 || limit > math.MaxInt32 {
		return fmt.Errorf("%w: offset and limit must fit PostgreSQL integer bounds", domain.ErrInvalidPagination)
	}
	return nil
}

func deploymentsFromRows(rows []postgres.Deployment) ([]domain.Deployment, error) {
	deployments := make([]domain.Deployment, 0, len(rows))
	for _, row := range rows {
		deployment, err := deploymentFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("decode deployment %s: %w", uuid.UUID(row.ID.Bytes), err)
		}
		deployments = append(deployments, deployment)
	}
	return deployments, nil
}

func deploymentFromRow(row postgres.Deployment) (domain.Deployment, error) {
	savedStatus, err := domain.ParseStatus(row.Status)
	if err != nil {
		return domain.Deployment{}, fmt.Errorf("decode deployment status: %w", err)
	}
	var savedSpec domain.DeploymentSpec
	if err := json.Unmarshal(row.Spec, &savedSpec); err != nil {
		return domain.Deployment{}, fmt.Errorf("decode deployment spec: %w", err)
	}
	return domain.Deployment{
		ID:             uuid.UUID(row.ID.Bytes),
		ContainerID:    row.ContainerID,
		ModelID:        uuid.UUID(row.ModelID.Bytes),
		Project:        row.Project,
		Username:       row.Username,
		Name:           row.Name,
		DeploymentName: row.DeploymentName,
		Status:         savedStatus,
		Spec:           savedSpec,
	}, nil
}

func (r *DeploymentRepository) ListDeploymentStates(ctx context.Context) ([]domain.DeploymentState, error) {
	rows, err := r.queries.ListDeploymentStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list deployment states: %w", err)
	}
	states := make([]domain.DeploymentState, 0, len(rows))
	for _, row := range rows {
		status, err := domain.ParseStatus(row.Status)
		if err != nil {
			return nil, fmt.Errorf("decode deployment status: %w", err)
		}
		states = append(states, domain.DeploymentState{
			ID: uuid.UUID(row.ID.Bytes), ContainerID: row.ContainerID, Status: status,
			Project: row.Project, DeploymentName: row.DeploymentName, Name: row.Name,
		})
	}
	return states, nil
}

func (r *DeploymentRepository) GetDeploymentState(ctx context.Context, containerID string) (domain.DeploymentState, error) {
	row, err := r.queries.GetDeploymentState(ctx, containerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeploymentState{}, fmt.Errorf("%w: %w", domain.ErrDeploymentNotFound, err)
	}
	if err != nil {
		return domain.DeploymentState{}, fmt.Errorf("get deployment state: %w", err)
	}
	status, err := domain.ParseStatus(row.Status)
	if err != nil {
		return domain.DeploymentState{}, fmt.Errorf("decode deployment status: %w", err)
	}
	return domain.DeploymentState{
		ID: uuid.UUID(row.ID.Bytes), ContainerID: row.ContainerID, Status: status,
		Project: row.Project, DeploymentName: row.DeploymentName, Name: row.Name,
	}, nil
}

func (r *DeploymentRepository) UpdateDeploymentStatus(ctx context.Context, previous domain.DeploymentState, next domain.Status) (bool, error) {
	if err := previous.Status.Validate(); err != nil {
		return false, fmt.Errorf("validate previous deployment status: %w", err)
	}
	if err := next.Validate(); err != nil {
		return false, fmt.Errorf("validate new deployment status: %w", err)
	}
	count, err := r.queries.UpdateDeploymentStatus(ctx, postgres.UpdateDeploymentStatusParams{
		ID: pgtype.UUID{Bytes: previous.ID, Valid: true}, ContainerID: previous.ContainerID,
		ExpectedStatus: string(previous.Status), NewStatus: string(next),
	})
	if err != nil {
		return false, fmt.Errorf("update deployment status: %w", err)
	}
	return count == 1, nil
}
