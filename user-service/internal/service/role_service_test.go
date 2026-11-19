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

// Техника: классы эквивалентности и границы идентификатора роли.
func TestRoleUseCases(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		for _, method := range []string{"create", "get", "list", "update", "delete", "platform"} {
			for _, valid := range []bool{true, false} {
				t.Run(method+map[bool]string{true: "/valid", false: "/invalid"}[valid], func(t *testing.T) {
					allure.Wrap(t, func(allureCtx *allure.Context) {
						u := testutil.UserMother{}.Regular()
						role := testutil.RoleMother{}.Developer()
						calls := 0
						r := &testutil.MockRoleRepository{CreateFunc: func(context.Context, domain.UserID, domain.Role) (domain.Role, error) { calls++; return role, nil }, GetFunc: func(context.Context, domain.RoleID) (domain.Role, error) { calls++; return role, nil }, ListFunc: func(context.Context, string, int, int) ([]domain.Role, error) { calls++; return nil, nil }, UpdateFunc: func(context.Context, domain.UserID, domain.RoleID, domain.RoleUpdate) (domain.Role, error) {
							calls++
							return role, nil
						}, DeleteFunc: func(context.Context, domain.UserID, domain.RoleID) error { calls++; return nil }}
						svc := service.NewRoleService(r)
						platform := service.NewPlatformRoleService(&testutil.MockPlatformRoleRepository{SetPlatformRoleFunc: func(context.Context, domain.UserID, domain.UserID, domain.PlatformRole) (domain.User, error) {
							calls++
							return u, nil
						}})
						ctx := service.WithPrincipal(context.Background(), service.Principal{UserID: u.ID})
						id := domain.RoleID(1)
						name := "role"
						if !valid {
							id = 0
							name = ""
						}
						var err error
						switch method {
						case "create":
							_, err = svc.CreateRole(ctx, domain.Role{Name: name})
						case "get":
							_, err = svc.GetRole(ctx, id)
						case "list":
							limit := 1
							if !valid {
								limit = 101
							}
							_, err = svc.ListRoles(ctx, service.ListRolesInput{Limit: limit})
						case "update":
							_, err = svc.UpdateRole(ctx, id, domain.RoleUpdate{Name: &name})
						case "delete":
							err = svc.DeleteRole(ctx, id)
						case "platform":
							target := u.ID
							if !valid {
								target = "invalid"
							}
							_, err = platform.SetRole(ctx, target, domain.PlatformRoleAdmin)
						}
						if valid && (err != nil || calls != 1) {
							allureCtx.Fatalf("err=%v calls=%d", err, calls)
						}
						if !valid && (!errors.Is(err, service.ErrInvalidInput) || calls != 0) {
							allureCtx.Fatalf("err=%v calls=%d", err, calls)
						}
					})
				})
			}
		}
	})
}

// Техника: классы эквивалентности ошибок репозитория.
func TestRoleRepositoryErrors(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("repository unavailable")
		user := testutil.UserMother{}.Regular()
		ctx := service.WithPrincipal(context.Background(), service.Principal{UserID: user.ID})
		name := "role"
		repo := &testutil.MockRoleRepository{CreateFunc: func(context.Context, domain.UserID, domain.Role) (domain.Role, error) { return domain.Role{}, failure }, GetFunc: func(context.Context, domain.RoleID) (domain.Role, error) { return domain.Role{}, failure }, ListFunc: func(context.Context, string, int, int) ([]domain.Role, error) { return nil, failure }, UpdateFunc: func(context.Context, domain.UserID, domain.RoleID, domain.RoleUpdate) (domain.Role, error) {
			return domain.Role{}, failure
		}, DeleteFunc: func(context.Context, domain.UserID, domain.RoleID) error { return failure }}
		svc := service.NewRoleService(repo)
		for _, tc := range []struct {
			name string
			call func() error
		}{{"create", func() error { _, e := svc.CreateRole(ctx, domain.Role{Name: name}); return e }}, {"get", func() error { _, e := svc.GetRole(ctx, 1); return e }}, {"list", func() error { _, e := svc.ListRoles(ctx, service.ListRolesInput{}); return e }}, {"update", func() error { _, e := svc.UpdateRole(ctx, 1, domain.RoleUpdate{Name: &name}); return e }}, {"delete", func() error { return svc.DeleteRole(ctx, 1) }}} {
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
