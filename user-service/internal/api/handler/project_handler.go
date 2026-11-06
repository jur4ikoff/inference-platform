package handler

import (
	"context"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
	"net/http"
)

type projectService interface {
	ListProjects(context.Context, service.ListProjectsInput) (service.ProjectsPage, error)
	CreateProject(context.Context, string, string) (domain.Project, error)
	GetProject(context.Context, string) (domain.Project, error)
	UpdateProject(context.Context, string, *string) (domain.Project, error)
	DeleteProject(context.Context, string) error
	ListMembers(context.Context, string, int, int) (service.MembersPage, error)
	ChangeMember(context.Context, string, domain.UserID, domain.RoleID, bool) (bool, error)
}
type ProjectHandler struct{ projects projectService }

func NewProjectHandler(p projectService) *ProjectHandler { return &ProjectHandler{projects: p} }
func projectResponse(p domain.Project) ProjectResponse {
	return ProjectResponse{ID: string(p.ID), Name: p.Name, Description: p.Description}
}
func (h *ProjectHandler) CreateProject(ctx context.Context, in *CreateProjectInput) (*ProjectOutput, error) {
	p, e := h.projects.CreateProject(ctx, in.Body.Name, in.Body.Description)
	if e != nil {
		return nil, mapServiceError(e)
	}
	return &ProjectOutput{Body: projectResponse(p)}, nil
}
func (h *ProjectHandler) GetProject(ctx context.Context, in *GetProjectInput) (*ProjectOutput, error) {
	p, e := h.projects.GetProject(ctx, in.Name)
	if e != nil {
		return nil, mapServiceError(e)
	}
	return &ProjectOutput{Body: projectResponse(p)}, nil
}
func (h *ProjectHandler) UpdateProject(ctx context.Context, in *UpdateProjectInput) (*UpdateProjectOutput, error) {
	p, e := h.projects.UpdateProject(ctx, in.Name, in.Body.Description)
	if e != nil {
		return nil, mapServiceError(e)
	}
	return &UpdateProjectOutput{Body: projectResponse(p)}, nil
}
func (h *ProjectHandler) DeleteProject(ctx context.Context, in *DeleteProjectInput) (*DeleteProjectOutput, error) {
	if e := h.projects.DeleteProject(ctx, in.Name); e != nil {
		return nil, mapServiceError(e)
	}
	return &DeleteProjectOutput{}, nil
}
func (h *ProjectHandler) ListProjects(ctx context.Context, in *ListProjectsInput) (*ListProjectsOutput, error) {
	page, e := h.projects.ListProjects(ctx, service.ListProjectsInput{Name: in.Name, Offset: in.Offset, Limit: in.Limit})
	if e != nil {
		return nil, mapServiceError(e)
	}
	ps := make([]ProjectResponse, 0, len(page.Projects))
	for _, p := range page.Projects {
		ps = append(ps, projectResponse(p))
	}
	return &ListProjectsOutput{Body: ListProjectsResponse{Projects: ps, NextPageToken: page.NextPageToken}}, nil
}
func (h *ProjectHandler) ListMembers(ctx context.Context, in *ListMembersInput) (*ListMembersOutput, error) {
	p, e := h.projects.ListMembers(ctx, in.Name, in.Offset, in.Limit)
	if e != nil {
		return nil, mapServiceError(e)
	}
	out := &ListMembersOutput{}
	out.Body.Members = make([]MemberResponse, 0, len(p.Members))
	out.Body.NextPageToken = p.NextPageToken
	for _, m := range p.Members {
		out.Body.Members = append(out.Body.Members, MemberResponse{UserID: string(m.UserID), Email: m.Email, Firstname: m.Firstname, Lastname: m.Lastname, RoleID: int(m.RoleID), RoleName: m.RoleName})
	}
	return out, nil
}
func (h *ProjectHandler) PutMember(ctx context.Context, in *PutMemberInput) (*PutMemberOutput, error) {
	created, e := h.projects.ChangeMember(ctx, in.Name, domain.UserID(in.UserID), domain.RoleID(in.Body.RoleID), false)
	if e != nil {
		return nil, mapServiceError(e)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return &PutMemberOutput{Status: status}, nil
}
func (h *ProjectHandler) DeleteMember(ctx context.Context, in *MemberPathInput) (*DeleteMemberOutput, error) {
	_, e := h.projects.ChangeMember(ctx, in.Name, domain.UserID(in.UserID), 0, true)
	if e != nil {
		return nil, mapServiceError(e)
	}
	return &DeleteMemberOutput{}, nil
}
