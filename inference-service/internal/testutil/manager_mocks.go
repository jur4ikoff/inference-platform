package testutil

import (
	"context"
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/moby/moby/client"
)

type MockManagerRepository struct {
	ListDeploymentStatesFunc   func(context.Context) ([]domain.DeploymentState, error)
	GetDeploymentStateFunc     func(context.Context, string) (domain.DeploymentState, error)
	UpdateDeploymentStatusFunc func(context.Context, domain.DeploymentState, domain.Status) (bool, error)
}

func (m *MockManagerRepository) ListDeploymentStates(ctx context.Context) ([]domain.DeploymentState, error) {
	return m.ListDeploymentStatesFunc(ctx)
}
func (m *MockManagerRepository) GetDeploymentState(ctx context.Context, id string) (domain.DeploymentState, error) {
	return m.GetDeploymentStateFunc(ctx, id)
}
func (m *MockManagerRepository) UpdateDeploymentStatus(ctx context.Context, state domain.DeploymentState, status domain.Status) (bool, error) {
	return m.UpdateDeploymentStatusFunc(ctx, state, status)
}

type MockManagerRuntime struct {
	GetContainerFunc         func(context.Context, string) (client.ContainerInspectResult, error)
	WatchContainerEventsFunc func(context.Context, string) client.EventsResult
	ContainerEndpointFunc    func(client.ContainerInspectResult) (string, error)
}

func (m *MockManagerRuntime) GetContainer(ctx context.Context, id string) (client.ContainerInspectResult, error) {
	return m.GetContainerFunc(ctx, id)
}
func (m *MockManagerRuntime) WatchContainerEvents(ctx context.Context, since string) client.EventsResult {
	return m.WatchContainerEventsFunc(ctx, since)
}
func (m *MockManagerRuntime) ContainerEndpoint(info client.ContainerInspectResult) (string, error) {
	return m.ContainerEndpointFunc(info)
}

type MockRouteRegistry struct {
	UpsertFunc func(domain.DeploymentState, string) error
	RemoveFunc func(uuid.UUID)
	PruneFunc  func([]domain.DeploymentState)
}

func (m *MockRouteRegistry) Upsert(state domain.DeploymentState, endpoint string) error {
	return m.UpsertFunc(state, endpoint)
}
func (m *MockRouteRegistry) Remove(id uuid.UUID)                   { m.RemoveFunc(id) }
func (m *MockRouteRegistry) Prune(states []domain.DeploymentState) { m.PruneFunc(states) }
