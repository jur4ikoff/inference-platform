package testutil

import (
	"github.com/jur4ikoff/model-service/internal/domain"
	"time"
)

var fixtureTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type ModelBuilder struct{ value domain.Model }

func NewModelBuilder() *ModelBuilder {
	return &ModelBuilder{value: domain.Model{ID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", Name: "demo-model", Project: "demo", Username: "alice", CreatedAt: fixtureTime, UpdatedAt: fixtureTime, Spec: domain.ModelSpec{ProviderType: domain.ProviderHuggingFace, ProviderHuggingFace: &domain.HuggingFaceProvider{Repo: "org/model"}}}}
}
func (b *ModelBuilder) WithID(id string) *ModelBuilder           { b.value.ID = id; return b }
func (b *ModelBuilder) WithName(name string) *ModelBuilder       { b.value.Name = name; return b }
func (b *ModelBuilder) WithProject(project string) *ModelBuilder { b.value.Project = project; return b }
func (b *ModelBuilder) WithUsername(username string) *ModelBuilder {
	b.value.Username = username
	return b
}
func (b *ModelBuilder) WithRepository(repo string) *ModelBuilder {
	b.value.Spec.ProviderHuggingFace = &domain.HuggingFaceProvider{Repo: repo}
	return b
}
func (b *ModelBuilder) Build() domain.Model {
	result := b.value
	if result.Spec.ProviderHuggingFace != nil {
		provider := *result.Spec.ProviderHuggingFace
		result.Spec.ProviderHuggingFace = &provider
	}
	return result
}

type ModelMother struct{}

func (ModelMother) HuggingFace() domain.Model { return NewModelBuilder().Build() }
func DeploymentFixture() domain.Deployment {
	return domain.Deployment{ID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f2", ModelID: NewModelBuilder().Build().ID, Project: "demo", Name: "demo-model", Username: "alice", Status: "running"}
}

type DeploymentBuilder struct{ value domain.Deployment }

func NewDeploymentBuilder() *DeploymentBuilder { return &DeploymentBuilder{value: DeploymentFixture()} }
func (b *DeploymentBuilder) WithStatus(status string) *DeploymentBuilder {
	b.value.Status = status
	return b
}
func (b *DeploymentBuilder) WithProject(project string) *DeploymentBuilder {
	b.value.Project = project
	return b
}
func (b *DeploymentBuilder) WithModelID(id string) *DeploymentBuilder { b.value.ModelID = id; return b }
func (b *DeploymentBuilder) Build() domain.Deployment {
	result := b.value
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

func (DeploymentMother) Running() domain.Deployment { return NewDeploymentBuilder().Build() }
func (DeploymentMother) Stopped() domain.Deployment {
	return NewDeploymentBuilder().WithStatus("stopped").Build()
}
