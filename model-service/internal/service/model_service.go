package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jur4ikoff/model-service/internal/domain"
)

type ModelRepository interface {
	Create(ctx context.Context, model domain.Model) error
	Get(ctx context.Context, project, modelID string) (domain.Model, error)
	List(ctx context.Context, project string, offset, limit int) ([]domain.Model, error)
	Update(ctx context.Context, project, modelID string, name, description *string, spec json.RawMessage, updatedAt time.Time) (domain.Model, error)
	Delete(ctx context.Context, project, modelID string) error
}

type ModelRuntime interface {
	Start(ctx context.Context, model domain.Model) error
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
	UserID      string
	Project     string
	Spec        json.RawMessage
}

type UpdateModelInput struct {
	Name        *string
	Description *string
	Spec        json.RawMessage
}

func (s *ModelService) Create(ctx context.Context, input CreateModelInput) (domain.Model, error) {
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.Project) == "" || !validJSON(input.Spec) {
		return domain.Model{}, ErrInvalidInput
	}
	now := s.now().UTC()
	model := domain.Model{
		ID:          uuid.NewString(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		UserID:      strings.TrimSpace(input.UserID),
		Project:     strings.TrimSpace(input.Project),
		Spec:        input.Spec,
	}
	if err := s.models.Create(ctx, model); err != nil {
		return domain.Model{}, err
	}
	return model, nil
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
		if name == "" {
			return domain.Model{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		input.Description = &description
	}
	if input.Spec != nil && !validJSON(input.Spec) {
		return domain.Model{}, ErrInvalidInput
	}
	return s.models.Update(ctx, project, modelID, input.Name, input.Description, input.Spec, s.now().UTC())
}

func (s *ModelService) Delete(ctx context.Context, project, modelID string) error {
	project, modelID = strings.TrimSpace(project), strings.TrimSpace(modelID)
	if project == "" || !validModelID(modelID) {
		return ErrInvalidInput
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

func validJSON(value json.RawMessage) bool {
	return len(value) > 0 && json.Valid(value)
}

func validModelID(modelID string) bool {
	_, err := uuid.Parse(modelID)
	return err == nil
}
