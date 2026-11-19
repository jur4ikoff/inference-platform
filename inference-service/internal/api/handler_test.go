package api

import (
	"context"
	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/service"
	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

// Техника: разбиение на классы эквивалентности; случай существующего deployment.
func TestStartExistingDeploymentReturnsAlreadyExists(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		svc := service.NewDeploymentService(existingDeploymentRepository{}, config.ServiceConfig{}, nil)
		response, err := NewHandler(svc).StartDeployment(context.Background(), validStartDeploymentRequest())
		if response != nil || status.Code(err) != codes.AlreadyExists {
			allureCtx.Fatalf("expected AlreadyExists without calling Docker, got response=%v error=%v", response, err)
		}
	})
}

type existingDeploymentRepository struct {
	service.DeploymentRepository
}

func (existingDeploymentRepository) ActiveDeploymentExists(context.Context, string, uuid.UUID) (bool, error) {
	return true, nil
}

// Техника: разбиение на классы эквивалентности; случай временных меток при сериализации.
func TestDeploymentTimestampsProtoRoundTrip(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		updated := started.Add(time.Minute)
		response := deploymentResponse(domain.Deployment{StartedAt: &started, UpdatedAt: &updated})
		data, err := proto.Marshal(response)
		if err != nil {
			allureCtx.Fatal(err)
		}
		var decoded inferencev1.Deployment
		if err := proto.Unmarshal(data, &decoded); err != nil {
			allureCtx.Fatal(err)
		}
		if !decoded.GetStartedAt().AsTime().Equal(started) || !decoded.GetUpdatedAt().AsTime().Equal(updated) {
			allureCtx.Fatalf("timestamps lost: %+v", &decoded)
		}
		if got := deploymentResponse(domain.Deployment{}); got.StartedAt != nil || got.UpdatedAt != nil {
			allureCtx.Fatal("unknown timestamps must remain unset")
		}
	})
}
