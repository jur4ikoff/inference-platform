package api

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/service"
)

type modelUseCases interface {
	Create(context.Context, service.CreateModelInput) (domain.Model, error)
	Get(context.Context, string, string) (domain.Model, error)
	List(context.Context, service.ListModelsInput) (service.ModelsPage, error)
	Update(context.Context, string, string, service.UpdateModelInput) (domain.Model, error)
	Delete(context.Context, string, string) error
}

type Handler struct {
	models modelUseCases
}

func NewHandler(models modelUseCases) *Handler {
	return &Handler{models: models}
}

func (h *Handler) createModel(ctx context.Context, input *createModelInput) (*modelOutput, error) {
	model, err := h.models.Create(ctx, service.CreateModelInput{
		Name: input.Body.Name, Description: input.Body.Description, UserID: input.Body.UserID, Project: input.Project, Spec: input.Body.Spec,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &modelOutput{Body: modelResponse(model)}, nil
}

func (h *Handler) listModels(ctx context.Context, input *listModelsInput) (*listModelsOutput, error) {
	page, err := h.models.List(ctx, service.ListModelsInput{
		Project: input.Project, Offset: input.Offset, Limit: input.Limit,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := make([]modelResponseBody, 0, len(page.Models))
	for _, model := range page.Models {
		response = append(response, modelResponse(model))
	}
	return &listModelsOutput{Body: listModelsResponse{Models: response, NextPageToken: page.NextPageToken}}, nil
}

func (h *Handler) getModel(ctx context.Context, input *modelInput) (*modelOutput, error) {
	model, err := h.models.Get(ctx, input.Project, input.ModelID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &modelOutput{Body: modelResponse(model)}, nil
}

func (h *Handler) updateModel(ctx context.Context, input *updateModelInput) (*modelOutput, error) {
	model, err := h.models.Update(ctx, input.Project, input.ModelID, service.UpdateModelInput{
		Name: input.Body.Name, Description: input.Body.Description, Spec: input.Body.Spec,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &modelOutput{Body: modelResponse(model)}, nil
}

func (h *Handler) deleteModel(ctx context.Context, input *modelInput) (*struct{}, error) {
	if err := h.models.Delete(ctx, input.Project, input.ModelID); err != nil {
		return nil, mapServiceError(err)
	}
	return &struct{}{}, nil
}

func modelResponse(model domain.Model) modelResponseBody {
	return modelResponseBody{ID: model.ID, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt, Name: model.Name, Description: model.Description, UserID: model.UserID, Project: model.Project, Spec: model.Spec}
}

func mapServiceError(err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return huma.Error400BadRequest("invalid input")
	case errors.Is(err, domain.ErrModelNotFound):
		return huma.Error404NotFound("model not found")
	case errors.Is(err, domain.ErrModelAlreadyExists):
		return huma.Error409Conflict("model name already exists in project")
	case errors.Is(err, service.ErrAlreadyRunning):
		return huma.Error409Conflict("model is already running")
	case errors.Is(err, service.ErrNotRunning):
		return huma.Error409Conflict("model is not running")
	case errors.Is(err, service.ErrRuntimeUnavailable):
		return huma.Error503ServiceUnavailable("model runtime is unavailable")
	case errors.Is(err, context.DeadlineExceeded):
		return huma.Error504GatewayTimeout("model runtime request timed out")
	default:
		return huma.Error500InternalServerError("internal server error")
	}
}
