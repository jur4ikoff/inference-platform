package api

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/service"
	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	maxInputBytes  = 1 << 20
	maxOutputBytes = 10 << 20
)

type Handler struct {
	inferencev1.UnimplementedInferenceServiceServer
	inferenceService *service.DeploymentService
}

var _ inferencev1.InferenceServiceServer = (*Handler)(nil)

func NewHandler(inference *service.DeploymentService) *Handler {
	return &Handler{
		inferenceService: inference,
	}
}

func (h *Handler) StartDeployment(ctx context.Context, request *inferencev1.StartDeploymentRequest) (*inferencev1.StartDeploymentResponse, error) {
	command, err := startDeploymentCommand(request)
	if err != nil {
		return nil, err
	}
	state, err := h.inferenceService.StartDeployment(ctx, command)
	if err != nil {
		return nil, serviceError(err)
	}
	return &inferencev1.StartDeploymentResponse{State: modelState(state)}, nil
}

func (h *Handler) StopDeployment(ctx context.Context, request *inferencev1.StopDeploymentRequest) (*inferencev1.StopDeploymentResponse, error) {
	id, err := uuid.Parse(request.GetDeploymentId())
	if err != nil || id == uuid.Nil {
		return nil, status.Error(codes.InvalidArgument, domain.ErrInvalidDeploymentID.Error())
	}
	if err := h.inferenceService.StopDeployment(ctx, id); err != nil {
		return nil, serviceError(err)
	}
	return &inferencev1.StopDeploymentResponse{}, nil
}

func (h *Handler) GetDeploymentByID(ctx context.Context, request *inferencev1.GetDeploymentByIDRequest) (*inferencev1.GetDeploymentByIDResponse, error) {
	id, err := uuid.Parse(request.GetDeploymentId())
	if err != nil || id == uuid.Nil {
		return nil, status.Error(codes.InvalidArgument, domain.ErrInvalidDeploymentID.Error())
	}
	deployment, err := h.inferenceService.GetDeploymentByID(ctx, id)
	if err != nil {
		return nil, serviceError(err)
	}
	return &inferencev1.GetDeploymentByIDResponse{Deployment: deploymentResponse(deployment)}, nil
}

func (h *Handler) GetDeploymentByName(ctx context.Context, request *inferencev1.GetDeploymentByNameRequest) (*inferencev1.GetDeploymentByNameResponse, error) {
	if strings.TrimSpace(request.GetName()) == "" {
		return nil, status.Error(codes.InvalidArgument, domain.ErrInvalidDeploymentName.Error())
	}
	deployment, err := h.inferenceService.GetDeploymentByName(ctx, request.GetName())
	if err != nil {
		return nil, serviceError(err)
	}
	return &inferencev1.GetDeploymentByNameResponse{Deployment: deploymentResponse(deployment)}, nil
}

func (h *Handler) ListDeployments(ctx context.Context, request *inferencev1.ListDeploymentsRequest) (*inferencev1.ListDeploymentsResponse, error) {
	page, err := h.inferenceService.ListDeployments(ctx, service.ListDeploymentsInput{
		Offset: int(request.GetOffset()), Limit: int(request.GetLimit()), NextPageToken: request.GetNextPageToken(),
	})
	if err != nil {
		return nil, serviceError(err)
	}
	return &inferencev1.ListDeploymentsResponse{Deployments: deploymentResponses(page.Deployments), NextPageToken: page.NextPageToken}, nil
}

func (h *Handler) ListAllDeployments(ctx context.Context, request *inferencev1.ListAllDeploymentsRequest) (*inferencev1.ListAllDeploymentsResponse, error) {
	page, err := h.inferenceService.ListAllDeployments(ctx, service.ListDeploymentsInput{
		Offset: int(request.GetOffset()), Limit: int(request.GetLimit()), NextPageToken: request.GetNextPageToken(),
	})
	if err != nil {
		return nil, serviceError(err)
	}
	return &inferencev1.ListAllDeploymentsResponse{Deployments: deploymentResponses(page.Deployments), NextPageToken: page.NextPageToken}, nil
}

func deploymentResponses(deployments []domain.Deployment) []*inferencev1.Deployment {
	responses := make([]*inferencev1.Deployment, 0, len(deployments))
	for _, deployment := range deployments {
		responses = append(responses, deploymentResponse(deployment))
	}
	return responses
}

func validateSelector(project, modelID string) error {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(modelID) == "" {
		return status.Error(codes.InvalidArgument, "project and model_id are required")
	}
	return nil
}

func modelState(state domain.Deployment) *inferencev1.ModelState {
	return &inferencev1.ModelState{
		DeploymentId: state.ID.String(),
		ModelId:      state.ModelID.String(), Project: state.Project, DeploymentName: state.DeploymentName, Status: deploymentStatus(state.Status),
	}
}

func deploymentResponse(deployment domain.Deployment) *inferencev1.Deployment {
	spec := &inferencev1.DeploymentSpec{ProviderType: string(deployment.Spec.ProviderType)}
	if provider := deployment.Spec.ProviderHuggingFace; provider != nil {
		spec.ProviderHuggingFace = &inferencev1.HuggingFaceProvider{Repo: provider.Repo}
	}
	return &inferencev1.Deployment{
		DeploymentId:   deployment.ID.String(),
		ContainerId:    deployment.ContainerID,
		ModelId:        deployment.ModelID.String(),
		Project:        deployment.Project,
		Username:       deployment.Username,
		Name:           deployment.Name,
		DeploymentName: deployment.DeploymentName,
		Status:         deploymentStatus(deployment.Status),
		Spec:           spec,
	}
}

func deploymentStatus(value domain.Status) inferencev1.ModelStatus {
	switch value {
	case domain.StatusPending:
		return inferencev1.ModelStatus_MODEL_STATUS_PENDING
	case domain.StatusStarting:
		return inferencev1.ModelStatus_MODEL_STATUS_STARTING
	case domain.StatusStarted:
		return inferencev1.ModelStatus_MODEL_STATUS_STARTED
	case domain.StatusStopping:
		return inferencev1.ModelStatus_MODEL_STATUS_STOPPING
	case domain.StatusRunning:
		return inferencev1.ModelStatus_MODEL_STATUS_RUNNING
	case domain.StatusStopped:
		return inferencev1.ModelStatus_MODEL_STATUS_STOPPED
	case domain.StatusFailed:
		return inferencev1.ModelStatus_MODEL_STATUS_FAILED
	case domain.StatusUnknown:
		return inferencev1.ModelStatus_MODEL_STATUS_UNKNOWN
	default:
		return inferencev1.ModelStatus_MODEL_STATUS_UNSPECIFIED
	}
}

func serviceError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, domain.ErrDeploymentAlreadyExists):
		return status.Error(codes.AlreadyExists, "active deployment already exists")
	case errors.Is(err, domain.ErrDatabaseUnavailable):
		return status.Error(codes.Unavailable, "database is unavailable")
	case errors.Is(err, domain.ErrDeploymentNotFound):
		return status.Error(codes.NotFound, "deployment not found")
	case errors.Is(err, domain.ErrInvalidDeploymentID):
		return status.Error(codes.InvalidArgument, domain.ErrInvalidDeploymentID.Error())
	case errors.Is(err, domain.ErrInvalidDeploymentName):
		return status.Error(codes.InvalidArgument, domain.ErrInvalidDeploymentName.Error())
	case errors.Is(err, domain.ErrInvalidPagination):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal server error")
	}
}
