package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jur4ikoff/model-service/internal/domain"
)

type ModelRepository interface {
	Create(ctx context.Context, model domain.Model) (domain.Model, error)
	Get(ctx context.Context, project, modelID string) (domain.Model, error)
	List(ctx context.Context, project string, offset, limit int) ([]domain.Model, error)
	Update(ctx context.Context, project, modelID string, name, description *string, spec *domain.ModelSpec, updatedAt time.Time) (domain.Model, error)
	Delete(ctx context.Context, project, modelID string) error
}

type ModelRuntime interface {
	Start(ctx context.Context, model domain.Model) error
	CreateDeployment(context.Context, domain.Model) (domain.Deployment, error)
	GetDeployment(context.Context, domain.Model) (domain.Deployment, error)
	GetDeploymentsHistory(context.Context, string) ([]domain.Deployment, error)
	GetActiveDeployments(context.Context, string) ([]domain.Deployment, error)
	StopDeployment(context.Context, domain.Deployment) error
}

type ModelService struct {
	models  ModelRepository
	runtime ModelRuntime
	now     func() time.Time
}

func NewModelService(models ModelRepository, runtime ModelRuntime) *ModelService {
	return &ModelService{models: models, runtime: runtime, now: time.Now}
}

type CreateModelInput struct {
	Name        string
	Description string
	Username    string
	Project     string
	Spec        domain.ModelSpec
}

type UpdateModelInput struct {
	Name        *string
	Description *string
	Spec        *domain.ModelSpec
}

func (s *ModelService) Create(ctx context.Context, input CreateModelInput) (domain.Model, error) {
	name, username, project := strings.TrimSpace(input.Name), strings.TrimSpace(input.Username), strings.TrimSpace(input.Project)
	if !domain.ValidModelName(name) || username == "" || project == "" || input.Spec.Validate() != nil {
		return domain.Model{}, ErrInvalidInput
	}
	now := s.now().UTC()
	model := domain.Model{
		ID:          uuid.NewString(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		Username:    username,
		Project:     project,
		Spec:        input.Spec,
	}
	saved, err := s.models.Create(ctx, model)
	if err != nil {
		return domain.Model{}, err
	}
	return saved, nil
}

func (s *ModelService) Get(ctx context.Context, project, modelID string) (domain.Model, error) {
	project, modelID = strings.TrimSpace(project), strings.TrimSpace(modelID)
	if project == "" || !validModelID(modelID) {
		return domain.Model{}, ErrInvalidInput
	}
	return s.models.Get(ctx, project, modelID)
}

func (s *ModelService) Update(ctx context.Context, project, modelID string, input UpdateModelInput) (domain.Model, error) {
	project, modelID = strings.TrimSpace(project), strings.TrimSpace(modelID)
	if project == "" || !validModelID(modelID) || (input.Name == nil && input.Description == nil && input.Spec == nil) {
		return domain.Model{}, ErrInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if !domain.ValidModelName(name) {
			return domain.Model{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		input.Description = &description
	}
	var spec *domain.ModelSpec
	if input.Spec != nil {
		if err := input.Spec.Validate(); err != nil {
			return domain.Model{}, ErrInvalidInput
		}
		spec = input.Spec
	}
	return s.models.Update(ctx, project, modelID, input.Name, input.Description, spec, s.now().UTC())
}

func (s *ModelService) Delete(ctx context.Context, project, modelID string) error {
	project, modelID = strings.TrimSpace(project), strings.TrimSpace(modelID)
	if project == "" || !validModelID(modelID) {
		return ErrInvalidInput
	}
	if _, err := s.models.Get(ctx, project, modelID); err != nil {
		return err
	}
	if s.runtime == nil {
		return ErrRuntimeUnavailable
	}
	deployments, err := s.runtime.GetDeploymentsHistory(ctx, project)
	if err != nil {
		return err
	}
	for _, deployment := range deployments {
		if deployment.Project == project && deployment.ModelID == modelID && deployment.Status != "stopped" && deployment.Status != "failed" {
			return ErrActiveDeployments
		}
	}
	return s.models.Delete(ctx, project, modelID)
}

func (s *ModelService) Start(ctx context.Context, project, modelID string) (domain.ModelState, error) {
	model, err := s.Get(ctx, project, modelID)
	if err != nil {
		return domain.ModelState{}, err
	}
	if err := s.runtime.Start(ctx, model); err != nil {
		return domain.ModelState{}, err
	}
	return domain.ModelState{ModelID: model.ID, Project: model.Project, Name: model.Name, Status: "running"}, nil
}

func (s *ModelService) CreateDeployment(ctx context.Context, project, modelID string) (domain.Deployment, error) {
	model, err := s.Get(ctx, project, modelID)
	if err != nil {
		return domain.Deployment{}, err
	}
	if s.runtime == nil {
		return domain.Deployment{}, ErrRuntimeUnavailable
	}
	return s.runtime.CreateDeployment(ctx, model)
}

func (s *ModelService) GetDeployment(ctx context.Context, project, modelID string) (domain.Deployment, error) {
	model, err := s.Get(ctx, project, modelID)
	if err != nil {
		return domain.Deployment{}, err
	}
	if s.runtime == nil {
		return domain.Deployment{}, ErrRuntimeUnavailable
	}
	return s.runtime.GetDeployment(ctx, model)
}

func (s *ModelService) GetDeploymentsHistory(ctx context.Context, project string) ([]domain.Deployment, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, ErrInvalidInput
	}
	if s.runtime == nil {
		return nil, ErrRuntimeUnavailable
	}
	return s.runtime.GetDeploymentsHistory(ctx, project)
}

func (s *ModelService) GetActiveDeployments(ctx context.Context, project string) ([]domain.Deployment, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, ErrInvalidInput
	}
	if s.runtime == nil {
		return nil, ErrRuntimeUnavailable
	}
	return s.runtime.GetActiveDeployments(ctx, project)
}

// StopDeployment returns true when an asynchronous stop request was accepted.
// Missing and terminal deployments are intentionally idempotent no-ops.
func (s *ModelService) StopDeployment(ctx context.Context, project, modelID string) (bool, error) {
	deployment, err := s.GetDeployment(ctx, project, modelID)
	if errors.Is(err, ErrDeploymentNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if deployment.Status == "stopped" || deployment.Status == "failed" {
		return false, nil
	}
	if err := s.runtime.StopDeployment(ctx, deployment); err != nil {
		return false, err
	}
	return true, nil
}

func validModelID(modelID string) bool {
	_, err := uuid.Parse(modelID)
	return err == nil
}
