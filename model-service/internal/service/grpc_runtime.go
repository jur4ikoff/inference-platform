package service

import (
	"context"
	"fmt"
	"time"

	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"github.com/jur4ikoff/model-service/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type GRPCModelRuntime struct {
	client inferencev1.InferenceServiceClient
}

func NewGRPCModelRuntime(client inferencev1.InferenceServiceClient) *GRPCModelRuntime {
	return &GRPCModelRuntime{client: client}
}

func (r *GRPCModelRuntime) Start(ctx context.Context, model domain.Model) error {
	rawSpec, err := model.Spec.CanonicalJSON()
	if err != nil {
		return fmt.Errorf("%w: decode model spec: %v", ErrInvalidInput, err)
	}
	spec := &inferencev1.DeploymentSpec{}
	if err := protojson.Unmarshal(rawSpec, spec); err != nil {
		return fmt.Errorf("%w: decode model spec: %v", ErrInvalidInput, err)
	}
	_, err = r.client.StartDeployment(ctx, &inferencev1.StartDeploymentRequest{
		Project:        model.Project,
		ModelId:        model.ID,
		Username:       model.Username,
		DeploymentName: model.Name,
		Spec:           spec,
	})
	return runtimeError(err)
}

func (r *GRPCModelRuntime) CreateDeployment(ctx context.Context, model domain.Model) (domain.Deployment, error) {
	rawSpec, err := model.Spec.CanonicalJSON()
	if err != nil {
		return domain.Deployment{}, fmt.Errorf("%w: decode model spec: %v", ErrInvalidInput, err)
	}
	spec := &inferencev1.DeploymentSpec{}
	if err := protojson.Unmarshal(rawSpec, spec); err != nil {
		return domain.Deployment{}, fmt.Errorf("%w: decode model spec: %v", ErrInvalidInput, err)
	}
	response, err := r.client.StartDeployment(ctx, &inferencev1.StartDeploymentRequest{
		Project:        model.Project,
		ModelId:        model.ID,
		Username:       model.Username,
		DeploymentName: model.Name,
		Spec:           spec,
	})
	if err != nil {
		return domain.Deployment{}, runtimeError(err)
	}
	state := response.GetState()
	if state == nil || state.GetDeploymentId() == "" {
		return domain.Deployment{}, fmt.Errorf("call model runtime: empty deployment state")
	}
	return deploymentFromState(state), nil
}

func (r *GRPCModelRuntime) GetDeployment(ctx context.Context, model domain.Model) (domain.Deployment, error) {
	deployments, err := r.GetDeploymentsHistory(ctx, model.Project)
	if err != nil {
		return domain.Deployment{}, err
	}
	for _, deployment := range deployments {
		if deployment.Project == model.Project && deployment.ModelID == model.ID {
			return deployment, nil
		}
	}
	return domain.Deployment{}, ErrDeploymentNotFound
}

func (r *GRPCModelRuntime) StopDeployment(ctx context.Context, deployment domain.Deployment) error {
	_, err := r.client.StopDeployment(ctx, &inferencev1.StopDeploymentRequest{DeploymentId: deployment.ID})
	return deploymentRuntimeError(err)
}

func (r *GRPCModelRuntime) GetDeploymentsHistory(ctx context.Context, project string) ([]domain.Deployment, error) {
	deployments := make([]domain.Deployment, 0)
	seen := make(map[string]bool)
	token := ""
	for {
		response, err := r.client.ListAllDeployments(ctx, &inferencev1.ListAllDeploymentsRequest{Limit: 100, NextPageToken: token})
		if err != nil {
			return nil, runtimeError(err)
		}
		for _, item := range response.GetDeployments() {
			if item == nil || item.GetProject() != project {
				continue
			}
			deployments = append(deployments, domain.Deployment{
				ID: item.GetDeploymentId(), ModelID: item.GetModelId(), Project: item.GetProject(),
				Name: item.GetDeploymentName(), Username: item.GetUsername(), Status: deploymentStatus(item.GetStatus()), StartedAt: protoTime(item.GetStartedAt()), UpdatedAt: protoTime(item.GetUpdatedAt()),
			})
		}
		token = response.GetNextPageToken()
		if token == "" {
			return deployments, nil
		}
		if seen[token] {
			return nil, fmt.Errorf("call model runtime: repeated pagination token")
		}
		seen[token] = true
	}
}

func (r *GRPCModelRuntime) GetActiveDeployments(ctx context.Context, project string) ([]domain.Deployment, error) {
	deployments := make([]domain.Deployment, 0)
	seen := make(map[string]bool)
	token := ""
	for {
		response, err := r.client.ListDeployments(ctx, &inferencev1.ListDeploymentsRequest{Limit: 100, NextPageToken: token})
		if err != nil {
			return nil, runtimeError(err)
		}
		for _, item := range response.GetDeployments() {
			if item == nil || item.GetProject() != project {
				continue
			}
			deployments = append(deployments, domain.Deployment{ID: item.GetDeploymentId(), ModelID: item.GetModelId(), Project: item.GetProject(), Name: item.GetDeploymentName(), Username: item.GetUsername(), Status: deploymentStatus(item.GetStatus()), StartedAt: protoTime(item.GetStartedAt()), UpdatedAt: protoTime(item.GetUpdatedAt())})
		}
		token = response.GetNextPageToken()
		if token == "" {
			return deployments, nil
		}
		if seen[token] {
			return nil, fmt.Errorf("call model runtime: repeated pagination token")
		}
		seen[token] = true
	}
}

func protoTime(value *timestamppb.Timestamp) *time.Time {
	if value == nil || !value.IsValid() {
		return nil
	}
	t := value.AsTime().UTC()
	return &t
}

func deploymentFromState(state *inferencev1.ModelState) domain.Deployment {
	return domain.Deployment{
		ID: state.GetDeploymentId(), ModelID: state.GetModelId(), Project: state.GetProject(),
		Name: state.GetDeploymentName(), Status: deploymentStatus(state.GetStatus()),
	}
}

func deploymentStatus(value inferencev1.ModelStatus) string {
	switch value {
	case inferencev1.ModelStatus_MODEL_STATUS_PENDING:
		return "pending"
	case inferencev1.ModelStatus_MODEL_STATUS_STARTING:
		return "starting"
	case inferencev1.ModelStatus_MODEL_STATUS_STARTED, inferencev1.ModelStatus_MODEL_STATUS_RUNNING:
		return "running"
	case inferencev1.ModelStatus_MODEL_STATUS_STOPPING:
		return "stopping"
	case inferencev1.ModelStatus_MODEL_STATUS_STOPPED:
		return "stopped"
	case inferencev1.ModelStatus_MODEL_STATUS_FAILED:
		return "failed"
	default:
		return "unknown"
	}
}

func runtimeError(err error) error {
	if err == nil {
		return nil
	}
	var cause error
	switch status.Code(err) {
	case codes.InvalidArgument:
		cause = ErrInvalidInput
	case codes.NotFound:
		cause = domain.ErrModelNotFound
	case codes.AlreadyExists:
		cause = ErrAlreadyRunning
	case codes.FailedPrecondition:
		cause = ErrNotRunning
	case codes.Unavailable:
		cause = ErrRuntimeUnavailable
	case codes.DeadlineExceeded:
		cause = context.DeadlineExceeded
	case codes.Canceled:
		cause = context.Canceled
	default:
		return fmt.Errorf("call model runtime: %w", err)
	}
	return fmt.Errorf("%w: %v", cause, err)
}

func deploymentRuntimeError(err error) error {
	if status.Code(err) == codes.NotFound {
		return fmt.Errorf("%w: %v", ErrDeploymentNotFound, err)
	}
	return runtimeError(err)
}
