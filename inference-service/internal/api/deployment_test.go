package api

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStartExistingDeploymentReturnsAlreadyExists(t *testing.T) {
	svc := service.NewDeploymentService(existingDeploymentRepository{}, config.ServiceConfig{}, nil)
	response, err := NewHandler(svc).StartDeployment(context.Background(), validStartDeploymentRequest())
	if response != nil || status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists without calling Docker, got response=%v error=%v", response, err)
	}
}

type existingDeploymentRepository struct {
	service.DeploymentRepository
}

func (existingDeploymentRepository) ActiveDeploymentExists(context.Context, string, uuid.UUID) (bool, error) {
	return true, nil
}
