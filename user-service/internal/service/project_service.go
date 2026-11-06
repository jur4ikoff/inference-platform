package service

import (
	"context"
	"math"
	"regexp"
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

type ListProjectsInput struct {
	Name   string
	Offset int
	Limit  int
}

type ProjectsPage struct {
	Projects      []domain.Project
	NextPageToken *int
}

func (s *ProjectService) ListProjects(ctx context.Context, input ListProjectsInput) (ProjectsPage, error) {
	actor, err := projectActor(ctx)
	if err != nil {
		return ProjectsPage{}, err
	}
	limit, err := normalizePageLimit(input.Offset, input.Limit)
	if err != nil {
		return ProjectsPage{}, err
	}
	input.Limit = limit
	input.Name = strings.TrimSpace(input.Name)
	projects, err := s.projects.List(ctx, actor, input.Name, input.Offset, input.Limit+1)
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

var projectNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

func validProjectName(name string) bool {
	return len(name) > 0 && len(name) <= 63 && projectNamePattern.MatchString(name)
}
func projectActor(ctx context.Context) (domain.UserID, error) {
	p, ok := PrincipalFromContext(ctx)
	if !ok {
		return "", ErrInvalidToken
	}
	return p.UserID, nil
}
func (s *ProjectService) CreateProject(ctx context.Context, name, description string) (domain.Project, error) {
	a, e := projectActor(ctx)
	if e != nil {
		return domain.Project{}, e
	}
	if !validProjectName(name) {
		return domain.Project{}, ErrInvalidInput
	}
	return s.projects.Create(ctx, a, name, description)
}
func (s *ProjectService) GetProject(ctx context.Context, name string) (domain.Project, error) {
	a, e := projectActor(ctx)
	if e != nil {
		return domain.Project{}, e
	}
	if !validProjectName(name) {
		return domain.Project{}, ErrInvalidInput
	}
	return s.projects.Get(ctx, a, name)
}
func (s *ProjectService) UpdateProject(ctx context.Context, name string, description *string) (domain.Project, error) {
	a, e := projectActor(ctx)
	if e != nil {
		return domain.Project{}, e
	}
	if !validProjectName(name) || description == nil {
		return domain.Project{}, ErrInvalidInput
	}
	return s.projects.Update(ctx, a, name, *description)
}
func (s *ProjectService) DeleteProject(ctx context.Context, name string) error {
	a, e := projectActor(ctx)
	if e != nil {
		return e
	}
	if !validProjectName(name) {
		return ErrInvalidInput
	}
	return s.projects.Delete(ctx, a, name)
}

type MembersPage struct {
	Members       []domain.Member
	NextPageToken *int
}

func (s *ProjectService) ListMembers(ctx context.Context, name string, offset, limit int) (MembersPage, error) {
	a, e := projectActor(ctx)
	if e != nil {
		return MembersPage{}, e
	}
	if !validProjectName(name) {
		return MembersPage{}, ErrInvalidInput
	}
	limit, e = normalizePageLimit(offset, limit)
	if e != nil {
		return MembersPage{}, e
	}
	m, e := s.projects.Members(ctx, a, name, offset, limit+1)
	if e != nil {
		return MembersPage{}, e
	}
	p := MembersPage{Members: m}
	if len(m) > limit {
		next := offset + limit
		p.NextPageToken = &next
		p.Members = m[:limit]
	}
	return p, nil
}
func (s *ProjectService) ChangeMember(ctx context.Context, name string, target domain.UserID, role domain.RoleID, remove bool) (bool, error) {
	a, e := projectActor(ctx)
	if e != nil {
		return false, e
	}
	if !validProjectName(name) || (!remove && (role <= 0 || role > math.MaxInt32)) {
		return false, ErrInvalidInput
	}
	return s.projects.ChangeMember(ctx, a, name, target, role, remove)
}
