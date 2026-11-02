package service

import (
	"context"
	"strings"

	"github.com/jur4ikoff/user-service/internal/domain"
)

// ProjectService contains project-related application use cases.
type ProjectService struct {
	projects ProjectRepository
}

func NewProjectService(projects ProjectRepository) *ProjectService {
	return &ProjectService{projects: projects}
}

type AddUserToProjectInput struct {
	ProjectID domain.ProjectID
	UserID    domain.UserID
	RoleID    domain.RoleID
}

// AddUserToProject creates a membership and assigns the supplied project role
// to the user.
func (s *ProjectService) AddUserToProject(ctx context.Context, input AddUserToProjectInput) error {
	if strings.TrimSpace(string(input.ProjectID)) == "" ||
		strings.TrimSpace(string(input.UserID)) == "" ||
		input.RoleID <= 0 {
		return ErrInvalidInput
	}

	return s.projects.AddUserToProject(ctx, input.ProjectID, input.UserID, input.RoleID)
}

type ListProjectsInput struct {
	Name   string
	Offset int
	Limit  int
}

type ProjectsPage struct {
	Projects      []domain.Project
	NextPageToken *int
}

func (s *ProjectService) ListProjects(ctx context.Context, userID domain.UserID, input ListProjectsInput) (ProjectsPage, error) {
	if strings.TrimSpace(string(userID)) == "" {
		return ProjectsPage{}, ErrInvalidInput
	}
	limit, err := normalizePageLimit(input.Offset, input.Limit)
	if err != nil {
		return ProjectsPage{}, err
	}
	input.Limit = limit
	input.Name = strings.TrimSpace(input.Name)
	projects, err := s.projects.List(ctx, userID, input.Name, input.Offset, input.Limit+1)
	if err != nil {
		return ProjectsPage{}, err
	}
	page := ProjectsPage{}
	if len(projects) > input.Limit {
		nextOffset := input.Offset + input.Limit
		page.NextPageToken = &nextOffset
		projects = projects[:input.Limit]
	}
	page.Projects = make([]domain.Project, len(projects))
	copy(page.Projects, projects)
	return page, nil
}
