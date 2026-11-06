// Package testutil provides fixtures and configurable test doubles.
package testutil

import (
	"context"
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/service"
)

// MockDeploymentRepository delegates each call to the corresponding Func field.
type MockDeploymentRepository struct {
	ActiveDeploymentExistsFunc func(ctx context.Context, project string, id uuid.UUID) (bool, error)
	CreateDeploymentFunc       func(ctx context.Context, deployment domain.Deployment) (domain.Deployment, error)
	GetDeploymentByIDFunc      func(ctx context.Context, id uuid.UUID) (domain.Deployment, error)
	GetDeploymentByNameFunc    func(ctx context.Context, name string) (domain.Deployment, error)
	ListDeploymentsFunc        func(ctx context.Context, offset, limit int) ([]domain.Deployment, error)
	ListAllDeploymentsFunc     func(ctx context.Context, offset, limit int) ([]domain.Deployment, error)
	UpdateDeploymentStatusFunc func(ctx context.Context, previous domain.DeploymentState, next domain.Status) (bool, error)
}

var _ service.DeploymentRepository = (*MockDeploymentRepository)(nil)

func (m *MockDeploymentRepository) ActiveDeploymentExists(ctx context.Context, project string, id uuid.UUID) (bool, error) {
	return m.ActiveDeploymentExistsFunc(ctx, project, id)
}
func (m *MockDeploymentRepository) CreateDeployment(ctx context.Context, deployment domain.Deployment) (domain.Deployment, error) {
	return m.CreateDeploymentFunc(ctx, deployment)
}
func (m *MockDeploymentRepository) GetDeploymentByID(ctx context.Context, id uuid.UUID) (domain.Deployment, error) {
	return m.GetDeploymentByIDFunc(ctx, id)
}
func (m *MockDeploymentRepository) GetDeploymentByName(ctx context.Context, name string) (domain.Deployment, error) {
	return m.GetDeploymentByNameFunc(ctx, name)
}
func (m *MockDeploymentRepository) ListDeployments(ctx context.Context, offset, limit int) ([]domain.Deployment, error) {
	return m.ListDeploymentsFunc(ctx, offset, limit)
}
func (m *MockDeploymentRepository) ListAllDeployments(ctx context.Context, offset, limit int) ([]domain.Deployment, error) {
	return m.ListAllDeploymentsFunc(ctx, offset, limit)
}
func (m *MockDeploymentRepository) UpdateDeploymentStatus(ctx context.Context, previous domain.DeploymentState, next domain.Status) (bool, error) {
	return m.UpdateDeploymentStatusFunc(ctx, previous, next)
}

// MockRuntimeClient delegates each call to the corresponding Func field.
type MockRuntimeClient struct {
	EnsureImageFunc     func(ctx context.Context, image string) error
	CreateContainerFunc func(ctx context.Context, image, name string, spec domain.DeploymentSpec) (string, error)
	StartContainerFunc  func(ctx context.Context, id string) error
	RemoveContainerFunc func(ctx context.Context, id string) error
	StopContainerFunc   func(ctx context.Context, id string) error
}

var _ service.RuntimeClient = (*MockRuntimeClient)(nil)

func (m *MockRuntimeClient) EnsureImage(ctx context.Context, image string) error {
	return m.EnsureImageFunc(ctx, image)
}
func (m *MockRuntimeClient) CreateContainer(ctx context.Context, image, name string, spec domain.DeploymentSpec) (string, error) {
	return m.CreateContainerFunc(ctx, image, name, spec)
}
func (m *MockRuntimeClient) StartContainer(ctx context.Context, id string) error {
	return m.StartContainerFunc(ctx, id)
}
func (m *MockRuntimeClient) RemoveContainer(ctx context.Context, id string) error {
	return m.RemoveContainerFunc(ctx, id)
}
func (m *MockRuntimeClient) StopContainer(ctx context.Context, id string) error {
	return m.StopContainerFunc(ctx, id)
}
