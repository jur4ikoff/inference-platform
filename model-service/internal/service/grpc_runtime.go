package service

import (
	"context"
	"fmt"

	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"github.com/jur4ikoff/model-service/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

type GRPCModelRuntime struct {
	client inferencev1.InferenceServiceClient
}

func NewGRPCModelRuntime(client inferencev1.InferenceServiceClient) *GRPCModelRuntime {
	return &GRPCModelRuntime{client: client}
}

func (r *GRPCModelRuntime) Start(ctx context.Context, model domain.Model) error {
	spec := &inferencev1.DeploymentSpec{}
	if err := protojson.Unmarshal(model.Spec, spec); err != nil {
		return fmt.Errorf("%w: decode model spec: %v", ErrInvalidInput, err)
	}
	_, err := r.client.StartDeployment(ctx, &inferencev1.StartDeploymentRequest{
		Project:        model.Project,
		ModelId:        model.ID,
		UserId:         model.UserID,
		DeploymentName: model.Name,
		Spec:           spec,
	})
	return runtimeError(err)
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
