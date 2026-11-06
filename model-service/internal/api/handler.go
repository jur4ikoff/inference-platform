package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jur4ikoff/model-service/internal/auth"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/service"
)

type modelUseCases interface {
	Create(context.Context, service.CreateModelInput) (domain.Model, error)
	Get(context.Context, string, string) (domain.Model, error)
	List(context.Context, service.ListModelsInput) (service.ModelsPage, error)
	Update(context.Context, string, string, service.UpdateModelInput) (domain.Model, error)
	Delete(context.Context, string, string) error
	CreateDeployment(context.Context, string, string) (domain.Deployment, error)
	GetDeployment(context.Context, string, string) (domain.Deployment, error)
	GetDeploymentsHistory(context.Context, string) ([]domain.Deployment, error)
	GetActiveDeployments(context.Context, string) ([]domain.Deployment, error)
	StopDeployment(context.Context, string, string) (bool, error)
}

type Handler struct {
	models modelUseCases
	auth   auth.Verifier
}

func NewHandler(models modelUseCases, verifiers ...auth.Verifier) *Handler {
	handler := &Handler{models: models}
	if len(verifiers) > 0 {
		handler.auth = verifiers[0]
	}
	return handler
}

func (h *Handler) createModel(ctx context.Context, input *createModelInput) (*modelOutput, error) {
	spec := input.Body.Spec.toDomain()
	command := service.CreateModelInput{
		Name: input.Body.Name, Description: input.Body.Description, Project: input.Project, Spec: *spec,
	}
	if principal, ok := principalFromContext(ctx); ok {
		command.Username = principal.Username
	}
	model, err := h.models.Create(ctx, command)
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
	response := make([]listedModelResponse, 0, len(page.Models))
	for _, model := range page.Models {
		status := page.Statuses[model.ID]
		if status == "" {
			status = "unknown"
		}
		response = append(response, listedModelResponse{modelResponseBody: modelResponse(model), Status: status})
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
	var spec *domain.ModelSpec
	if input.Body.Spec != nil {
		spec = input.Body.Spec.toDomain()
	}
	model, err := h.models.Update(ctx, input.Project, input.ModelID, service.UpdateModelInput{
		Name: input.Body.Name, Description: input.Body.Description, Spec: spec,
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

type createDeploymentOutput struct {
	Status   int    `status:"202"`
	Location string `header:"Location" doc:"URL созданного развёртывания"`
	Body     deploymentResponse
}

type deploymentOutput struct {
	Body deploymentResponse
}

type deploymentsHistoryInput struct {
	Project string `path:"project" minLength:"1" doc:"Имя проекта"`
}

type deploymentsHistoryOutput struct {
	Body struct {
		Deployments []deploymentHistoryResponse `json:"deployments"`
	}
}

type deploymentHistoryResponse struct {
	deploymentResponse
	Name      string     `json:"name"`
	Username  string     `json:"username"`
	StartedAt *time.Time `json:"started_at" doc:"Время создания deployment; null, если неизвестно"`
	UpdatedAt *time.Time `json:"updated_at" doc:"Время последнего изменения статуса; null, если неизвестно"`
}

func (h *Handler) getDeploymentsHistory(ctx context.Context, input *deploymentsHistoryInput) (*deploymentsHistoryOutput, error) {
	deployments, err := h.models.GetDeploymentsHistory(ctx, input.Project)
	if err != nil {
		return nil, mapServiceError(err)
	}
	output := &deploymentsHistoryOutput{}
	output.Body.Deployments = make([]deploymentHistoryResponse, 0, len(deployments))
	for _, d := range deployments {
		output.Body.Deployments = append(output.Body.Deployments, deploymentHistoryResponse{
			deploymentResponse: deploymentResponseBody(d.ID, d.ModelID, d.Project, d.Status), Name: d.Name, Username: d.Username, StartedAt: d.StartedAt, UpdatedAt: d.UpdatedAt,
		})
	}
	return output, nil
}

func (h *Handler) getActiveDeployments(ctx context.Context, input *deploymentsHistoryInput) (*deploymentsHistoryOutput, error) {
	deployments, err := h.models.GetActiveDeployments(ctx, input.Project)
	if err != nil {
		return nil, mapServiceError(err)
	}
	output := &deploymentsHistoryOutput{}
	output.Body.Deployments = make([]deploymentHistoryResponse, 0, len(deployments))
	for _, d := range deployments {
		output.Body.Deployments = append(output.Body.Deployments, deploymentHistoryResponse{deploymentResponse: deploymentResponseBody(d.ID, d.ModelID, d.Project, d.Status), Name: d.Name, Username: d.Username, StartedAt: d.StartedAt, UpdatedAt: d.UpdatedAt})
	}
	return output, nil
}

type deploymentResponse struct {
	ID      string `json:"id" format:"uuid" doc:"Идентификатор deployment"`
	ModelID string `json:"model_id" format:"uuid" doc:"Идентификатор развёрнутой модели"`
	Project string `json:"project" doc:"Имя проекта"`
	Status  string `json:"status" enum:"pending,starting,running,stopping,stopped,failed,unknown" doc:"Текущее состояние развёртывания"`
}

type deleteDeploymentOutput struct {
	Status int
}

func (h *Handler) createDeployment(ctx context.Context, input *modelInput) (*createDeploymentOutput, error) {
	deployment, err := h.models.CreateDeployment(ctx, input.Project, input.ModelID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &createDeploymentOutput{
		Status:   http.StatusAccepted,
		Location: apiPrefix + "/projects/" + input.Project + "/models/" + input.ModelID + "/deployment",
		Body:     deploymentResponseBody(deployment.ID, deployment.ModelID, deployment.Project, deployment.Status),
	}, nil
}

func (h *Handler) getDeployment(ctx context.Context, input *modelInput) (*deploymentOutput, error) {
	deployment, err := h.models.GetDeployment(ctx, input.Project, input.ModelID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &deploymentOutput{Body: deploymentResponseBody(deployment.ID, deployment.ModelID, deployment.Project, deployment.Status)}, nil
}

func (h *Handler) deleteDeployment(ctx context.Context, input *modelInput) (*deleteDeploymentOutput, error) {
	accepted, err := h.models.StopDeployment(ctx, input.Project, input.ModelID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	status := http.StatusNoContent
	if accepted {
		status = http.StatusAccepted
	}
	return &deleteDeploymentOutput{Status: status}, nil
}

func deploymentResponseBody(id, modelID, project, status string) deploymentResponse {
	return deploymentResponse{ID: id, ModelID: modelID, Project: project, Status: status}
}

func modelResponse(model domain.Model) modelResponseBody {
	return modelResponseBody{ID: model.ID, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt, Name: model.Name, Description: model.Description, Username: model.Username, Project: model.Project, Spec: modelSpecResponse(model.Spec)}
}

func mapServiceError(err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return huma.Error400BadRequest("invalid input")
	case errors.Is(err, domain.ErrModelNotFound):
		return huma.Error404NotFound("model not found")
	case errors.Is(err, domain.ErrModelAlreadyExists):
		return huma.Error409Conflict("model name already exists in project")
	case errors.Is(err, service.ErrActiveDeployments):
		return huma.Error409Conflict("model has deployments that are not stopped")
	case errors.Is(err, service.ErrAlreadyRunning):
		return huma.Error409Conflict("model is already running")
	case errors.Is(err, service.ErrNotRunning):
		return huma.Error409Conflict("model is not running")
	case errors.Is(err, service.ErrDeploymentNotFound):
		return huma.Error404NotFound("deployment not found")
	case errors.Is(err, service.ErrRuntimeUnavailable):
		return huma.Error503ServiceUnavailable("model runtime is unavailable")
	case errors.Is(err, context.DeadlineExceeded):
		return huma.Error504GatewayTimeout("model runtime request timed out")
	default:
		return huma.Error500InternalServerError("internal server error")
	}
}
