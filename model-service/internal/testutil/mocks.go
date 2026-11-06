// Package testutil provides fixtures and configurable test doubles.
package testutil

import (
	"context"
	"github.com/jur4ikoff/model-service/internal/domain"
	"time"
)

// MockModelRepository delegates each call to the corresponding Func field.
type MockModelRepository struct {
	CreateFunc func(ctx context.Context, model domain.Model) (domain.Model, error)
	GetFunc    func(ctx context.Context, project, id string) (domain.Model, error)
	ListFunc   func(ctx context.Context, project string, offset, limit int) ([]domain.Model, error)
	UpdateFunc func(ctx context.Context, project, id string, name, description *string, spec *domain.ModelSpec, updatedAt time.Time) (domain.Model, error)
	DeleteFunc func(ctx context.Context, project, id string) error
}

func (m *MockModelRepository) Create(ctx context.Context, model domain.Model) (domain.Model, error) {
	return m.CreateFunc(ctx, model)
}
func (m *MockModelRepository) Get(ctx context.Context, project, id string) (domain.Model, error) {
	return m.GetFunc(ctx, project, id)
}
func (m *MockModelRepository) List(ctx context.Context, project string, offset, limit int) ([]domain.Model, error) {
	return m.ListFunc(ctx, project, offset, limit)
}
func (m *MockModelRepository) Update(ctx context.Context, project, id string, name, description *string, spec *domain.ModelSpec, updatedAt time.Time) (domain.Model, error) {
	return m.UpdateFunc(ctx, project, id, name, description, spec, updatedAt)
}
func (m *MockModelRepository) Delete(ctx context.Context, project, id string) error {
	return m.DeleteFunc(ctx, project, id)
}

// MockModelRuntime delegates each call to the corresponding Func field.
type MockModelRuntime struct {
	StartFunc                 func(ctx context.Context, model domain.Model) error
	CreateDeploymentFunc      func(ctx context.Context, model domain.Model) (domain.Deployment, error)
	GetDeploymentFunc         func(ctx context.Context, model domain.Model) (domain.Deployment, error)
	GetDeploymentsHistoryFunc func(ctx context.Context, project string) ([]domain.Deployment, error)
	GetActiveDeploymentsFunc  func(ctx context.Context, project string) ([]domain.Deployment, error)
	StopDeploymentFunc        func(ctx context.Context, deployment domain.Deployment) error
}

func (m *MockModelRuntime) Start(ctx context.Context, model domain.Model) error {
	return m.StartFunc(ctx, model)
}
func (m *MockModelRuntime) CreateDeployment(ctx context.Context, model domain.Model) (domain.Deployment, error) {
	return m.CreateDeploymentFunc(ctx, model)
}
func (m *MockModelRuntime) GetDeployment(ctx context.Context, model domain.Model) (domain.Deployment, error) {
	return m.GetDeploymentFunc(ctx, model)
}
func (m *MockModelRuntime) GetDeploymentsHistory(ctx context.Context, project string) ([]domain.Deployment, error) {
	return m.GetDeploymentsHistoryFunc(ctx, project)
}
func (m *MockModelRuntime) GetActiveDeployments(ctx context.Context, project string) ([]domain.Deployment, error) {
	return m.GetActiveDeploymentsFunc(ctx, project)
}
func (m *MockModelRuntime) StopDeployment(ctx context.Context, deployment domain.Deployment) error {
	return m.StopDeploymentFunc(ctx, deployment)
}
