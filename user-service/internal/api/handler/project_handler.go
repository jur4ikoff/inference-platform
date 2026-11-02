package handler

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

type projectLister interface {
	ListProjects(context.Context, domain.UserID, service.ListProjectsInput) (service.ProjectsPage, error)
}

type ProjectHandler struct {
	projects projectLister
}

func NewProjectHandler(projects projectLister) *ProjectHandler {
	return &ProjectHandler{projects: projects}
}

func (h *ProjectHandler) CreateProject(context.Context, *CreateProjectInput) (*ProjectOutput, error) {
	return nil, huma.Error501NotImplemented("create project endpoint is not implemented")
}

func (h *ProjectHandler) GetProject(context.Context, *GetProjectInput) (*ProjectOutput, error) {
	return nil, huma.Error501NotImplemented("get project endpoint is not implemented")
}

func (h *ProjectHandler) UpdateProject(_ context.Context, input *UpdateProjectInput) (*UpdateProjectOutput, error) {
	if strings.TrimSpace(input.ID) == "" {
		return nil, huma.Error400BadRequest("project id is required")
	}
	if input.Body.Name == nil && input.Body.Description == nil {
		return nil, huma.Error400BadRequest("at least one field to update is required")
	}

	return nil, huma.Error501NotImplemented("update project endpoint is not implemented")
}

func (h *ProjectHandler) DeleteProject(_ context.Context, input *DeleteProjectInput) (*DeleteProjectOutput, error) {
	if strings.TrimSpace(input.ID) == "" {
		return nil, huma.Error400BadRequest("project id is required")
	}

	return nil, huma.Error501NotImplemented("delete project endpoint is not implemented")
}

func (h *ProjectHandler) ListProjects(ctx context.Context, input *ListProjectsInput) (*ListProjectsOutput, error) {
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("missing or invalid bearer token")
	}
	page, err := h.projects.ListProjects(ctx, principal.UserID, service.ListProjectsInput{
		Name: input.Name, Offset: input.Offset, Limit: input.Limit,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	projects := make([]ProjectResponse, 0, len(page.Projects))
	for _, project := range page.Projects {
		projects = append(projects, ProjectResponse{ID: string(project.ID), Name: project.Name, Description: project.Description})
	}
	return &ListProjectsOutput{Body: ListProjectsResponse{Projects: projects, NextPageToken: page.NextPageToken}}, nil
}
