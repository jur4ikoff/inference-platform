package testutil

import (
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/domain"
)

type DeploymentBuilder struct{ value domain.Deployment }

func NewDeploymentBuilder() *DeploymentBuilder {
	return &DeploymentBuilder{value: domain.Deployment{ID: uuid.MustParse("0197eb5a-37c6-7e48-8cd6-0c19edcd72f2"), ModelID: uuid.MustParse("0197eb5a-37c6-7e48-8cd6-0c19edcd72f1"), ContainerID: "container-1", Project: "demo", Username: "alice", Name: "demo-model-container", DeploymentName: "demo-model", Status: domain.StatusPending, Spec: domain.DeploymentSpec{ProviderType: domain.ModelProviderTypeHuggingFace, ProviderHuggingFace: &domain.HuggingFaceProvider{Repo: "org/model"}}}}
}
func (b *DeploymentBuilder) WithID(id uuid.UUID) *DeploymentBuilder { b.value.ID = id; return b }
func (b *DeploymentBuilder) WithModelID(id uuid.UUID) *DeploymentBuilder {
	b.value.ModelID = id
	return b
}
func (b *DeploymentBuilder) WithProject(project string) *DeploymentBuilder {
	b.value.Project = project
	return b
}
func (b *DeploymentBuilder) WithStatus(status domain.Status) *DeploymentBuilder {
	b.value.Status = status
	return b
}
func (b *DeploymentBuilder) WithRepository(repo string) *DeploymentBuilder {
	b.value.Spec.ProviderHuggingFace = &domain.HuggingFaceProvider{Repo: repo}
	return b
}
func (b *DeploymentBuilder) Build() domain.Deployment {
	result := b.value
	if result.Spec.ProviderHuggingFace != nil {
		provider := *result.Spec.ProviderHuggingFace
		result.Spec.ProviderHuggingFace = &provider
	}
	if result.StartedAt != nil {
		t := *result.StartedAt
		result.StartedAt = &t
	}
	if result.UpdatedAt != nil {
		t := *result.UpdatedAt
		result.UpdatedAt = &t
	}
	return result
}

type DeploymentMother struct{}

func (DeploymentMother) Pending() domain.Deployment { return NewDeploymentBuilder().Build() }
func (DeploymentMother) Running() domain.Deployment {
	return NewDeploymentBuilder().WithStatus(domain.StatusRunning).Build()
}
func (DeploymentMother) Stopped() domain.Deployment {
	return NewDeploymentBuilder().WithStatus(domain.StatusStopped).Build()
}
