package service_test

import (
	"context"
	"errors"
	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/user-service/internal/testutil"
	"testing"
)

// Техника: граничные значения и классы эквивалентности имён проекта.
func TestProjectNameValidation(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for _, tt := range []struct {
			name  string
			valid bool
		}{{"demo-project", true}, {"a", true}, {"Project", false}, {"demo--project", false}, {"-demo", false}, {"demo-", false}, {"", false}} {
			repository := &projectListStub{}
			projects := service.NewProjectService(repository)
			ctx := service.WithPrincipal(context.Background(), service.Principal{UserID: "actor-id"})
			_, err := projects.CreateProject(ctx, tt.name, "")
			if got := err == nil; got != tt.valid {
				allureCtx.Errorf("validProjectName(%q) = %t", tt.name, got)
			}
		}

	})
}

// Техника: классы эквивалентности авторизованного и анонимного пользователя.
func TestListProjectsUsesPrincipalFromContext(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for _, authenticated := range []bool{true, false} {
			t.Run(map[bool]string{true: "authenticated", false: "anonymous"}[authenticated], func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {

					repository := &projectListStub{}
					projects := service.NewProjectService(repository)
					actor := domain.UserID("actor-id")
					ctx := context.Background()
					if authenticated {
						ctx = service.WithPrincipal(ctx, service.Principal{UserID: actor})
					}
					_, err := projects.ListProjects(ctx, service.ListProjectsInput{})
					if authenticated && (err != nil || repository.actor != actor) {
						allureCtx.Fatalf("err=%v actor=%q", err, repository.actor)
					}
					if !authenticated && !errors.Is(err, service.ErrInvalidToken) {
						allureCtx.Fatalf("err=%v", err)
					}

				})
			})
		}

	})
}

type projectListStub struct {
	service.ProjectRepository
	actor domain.UserID
}

func (r *projectListStub) List(_ context.Context, actor domain.UserID, _ string, _, _ int) ([]domain.Project, error) {
	r.actor = actor
	return nil, nil
}

func (r *projectListStub) Create(_ context.Context, _ domain.UserID, name, description string) (domain.Project, error) {
	return domain.Project{Name: name, Description: description}, nil
}

// Техника: классы эквивалентности авторизованных и анонимных запросов.
func TestProjectUseCases(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for _, method := range []string{"create", "get", "list", "update", "delete", "members", "change member"} {
			for _, valid := range []bool{true, false} {
				t.Run(method+map[bool]string{true: "/valid", false: "/invalid"}[valid], func(t *testing.T) {
					allure.Wrap(t, func(allureCtx *allure.Context) {

						u := testutil.UserMother{}.Regular()
						p := testutil.ProjectMother{}.Demo()
						calls := 0
						r := &testutil.MockProjectRepository{CreateFunc: func(context.Context, domain.UserID, string, string) (domain.Project, error) { calls++; return p, nil }, GetFunc: func(context.Context, domain.UserID, string) (domain.Project, error) { calls++; return p, nil }, ListFunc: func(context.Context, domain.UserID, string, int, int) ([]domain.Project, error) {
							calls++
							return []domain.Project{p}, nil
						}, UpdateFunc: func(context.Context, domain.UserID, string, string) (domain.Project, error) { calls++; return p, nil }, DeleteFunc: func(context.Context, domain.UserID, string) error { calls++; return nil }, MembersFunc: func(context.Context, domain.UserID, string, int, int) ([]domain.Member, error) {
							calls++
							return nil, nil
						}, ChangeMemberFunc: func(context.Context, domain.UserID, string, domain.UserID, domain.RoleID, bool) (bool, error) {
							calls++
							return true, nil
						}}
						svc := service.NewProjectService(r)
						ctx := context.Background()
						if valid {
							ctx = service.WithPrincipal(ctx, service.Principal{UserID: u.ID})
						}
						desc := "description"
						var err error
						switch method {
						case "create":
							_, err = svc.CreateProject(ctx, "demo", desc)
						case "get":
							_, err = svc.GetProject(ctx, "demo")
						case "list":
							_, err = svc.ListProjects(ctx, service.ListProjectsInput{})
						case "update":
							_, err = svc.UpdateProject(ctx, "demo", &desc)
						case "delete":
							err = svc.DeleteProject(ctx, "demo")
						case "members":
							_, err = svc.ListMembers(ctx, "demo", 0, 1)
						case "change member":
							_, err = svc.ChangeMember(ctx, "demo", u.ID, 1, false)
						}
						if valid && (err != nil || calls != 1) {
							allureCtx.Fatalf("err=%v calls=%d", err, calls)
						}
						if !valid && (!errors.Is(err, service.ErrInvalidToken) || calls != 0) {
							allureCtx.Fatalf("err=%v calls=%d", err, calls)
						}

					})
				})
			}
		}

	})
}

// Техника: классы эквивалентности ошибок репозитория.
func TestProjectRepositoryErrors(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		failure := errors.New("repository unavailable")
		user := testutil.UserMother{}.Regular()
		ctx := service.WithPrincipal(context.Background(), service.Principal{UserID: user.ID})
		description := "description"
		repo := &testutil.MockProjectRepository{CreateFunc: func(context.Context, domain.UserID, string, string) (domain.Project, error) {
			return domain.Project{}, failure
		}, GetFunc: func(context.Context, domain.UserID, string) (domain.Project, error) { return domain.Project{}, failure }, ListFunc: func(context.Context, domain.UserID, string, int, int) ([]domain.Project, error) { return nil, failure }, UpdateFunc: func(context.Context, domain.UserID, string, string) (domain.Project, error) {
			return domain.Project{}, failure
		}, DeleteFunc: func(context.Context, domain.UserID, string) error { return failure }, MembersFunc: func(context.Context, domain.UserID, string, int, int) ([]domain.Member, error) { return nil, failure }, ChangeMemberFunc: func(context.Context, domain.UserID, string, domain.UserID, domain.RoleID, bool) (bool, error) {
			return false, failure
		}}
		svc := service.NewProjectService(repo)
		for _, tc := range []struct {
			name string
			call func() error
		}{{"create", func() error { _, e := svc.CreateProject(ctx, "demo", description); return e }}, {"get", func() error { _, e := svc.GetProject(ctx, "demo"); return e }}, {"list", func() error { _, e := svc.ListProjects(ctx, service.ListProjectsInput{}); return e }}, {"update", func() error { _, e := svc.UpdateProject(ctx, "demo", &description); return e }}, {"delete", func() error { return svc.DeleteProject(ctx, "demo") }}, {"members", func() error { _, e := svc.ListMembers(ctx, "demo", 0, 1); return e }}, {"change member", func() error { _, e := svc.ChangeMember(ctx, "demo", user.ID, 1, false); return e }}} {
			t.Run(tc.name, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {

					err := tc.call()
					if !errors.Is(err, failure) {
						allureCtx.Fatalf("err=%v", err)
					}

				})
			})
		}

	})
}
