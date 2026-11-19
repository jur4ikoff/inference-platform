package repository_test

import (
	"context"
	"errors"
	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/repository"
	"github.com/jur4ikoff/user-service/internal/testutil"
	"strings"
	"testing"
)

// Техника: классы эквивалентности результатов запроса и ошибок БД.
func TestProjectRepositoryUnit(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for _, method := range []string{"create", "list", "grant", "get", "update", "delete", "members", "change member"} {
			for _, scenario := range []string{"success", "database error"} {
				t.Run(method+"/"+scenario, func(t *testing.T) {
					allure.Wrap(t, func(allureCtx *allure.Context) {

						failure := errors.New("database error")
						var injected error
						if scenario == "database error" {
							injected = failure
						}
						db, _ := repositoryFixture(allureCtx, "", injected)
						repo := repository.NewProjectRepository(db)
						u := testutil.UserMother{}.Regular()
						ctx := context.Background()
						var err error
						switch method {
						case "create":
							_, err = repo.Create(ctx, u.ID, "demo", "description")
						case "list":
							_, err = repo.List(ctx, u.ID, "", 0, 10)
						case "grant":
							_, err = repo.ProjectTokenGrant(ctx, u.ID, "demo")
						case "get":
							_, err = repo.Get(ctx, u.ID, "demo")
						case "update":
							_, err = repo.Update(ctx, u.ID, "demo", "description")
						case "delete":
							err = repo.Delete(ctx, u.ID, "demo")
						case "members":
							_, err = repo.Members(ctx, u.ID, "demo", 0, 10)
						case "change member":
							_, err = repo.ChangeMember(ctx, u.ID, "demo", u.ID, domain.RoleID(1), false)
						}
						if scenario == "success" && err != nil {
							allureCtx.Fatalf("err=%v", err)
						}
						if scenario == "database error" && !errors.Is(err, failure) {
							allureCtx.Fatalf("err=%v", err)
						}

					})
				})
			}
		}

	})
}

// Техника: класс отсутствующего проекта.
func TestProjectRepositoryMissing(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for _, method := range []string{"grant", "get", "update", "delete", "members", "change member"} {
			t.Run(method, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {

					missing := "LockProject"
					if method == "grant" {
						missing = "GetProjectTokenGrant"
					}
					db, _ := repositoryFixture(allureCtx, missing, nil)
					r := repository.NewProjectRepository(db)
					id := testutil.UserMother{}.Regular().ID
					ctx := context.Background()
					var err error
					switch method {
					case "grant":
						_, err = r.ProjectTokenGrant(ctx, id, "demo")
					case "get":
						_, err = r.Get(ctx, id, "demo")
					case "update":
						_, err = r.Update(ctx, id, "demo", "desc")
					case "delete":
						err = r.Delete(ctx, id, "demo")
					case "members":
						_, err = r.Members(ctx, id, "demo", 0, 10)
					case "change member":
						_, err = r.ChangeMember(ctx, id, "demo", id, 1, false)
					}
					if !errors.Is(err, domain.ErrProjectNotFound) {
						allureCtx.Fatalf("err=%v", err)
					}

				})
			})
		}

	})
}

// Техника: класс существующего проекта и обработка результата.
func TestProjectRepositoryMapsResult(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		db, _ := repositoryFixture(allureCtx, "", nil)
		r := repository.NewProjectRepository(db)
		user := testutil.UserMother{}.Regular()
		project, err := r.Get(context.Background(), user.ID, "demo")
		if err != nil || project.Name != "demo" || project.Description != "description" {
			allureCtx.Fatalf("project=%+v err=%v", project, err)
		}

	})
}

// Техника: класс конфликта уникального имени проекта.
func TestProjectRepositoryDuplicate(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		db, tx := repositoryFixture(allureCtx, "", nil)
		original := tx.QueryRowFunc
		tx.QueryRowFunc = func(ctx context.Context, sql string, args ...interface{}) pgx.Row {
			if strings.Contains(sql, "-- name: InsertProject ") {
				return testutil.Row{Err: &pgconn.PgError{Code: "23505", ConstraintName: "projects_name_key"}}
			}
			return original(ctx, sql, args...)
		}
		user := testutil.UserMother{}.Regular()
		_, err := repository.NewProjectRepository(db).Create(context.Background(), user.ID, "demo", "")
		if !errors.Is(err, domain.ErrProjectExists) {
			allureCtx.Fatalf("err=%v", err)
		}

	})
}

// Техника: классы эквивалентности пустого списка и отсутствующего участника.
func TestProjectRepositoryEmptyResults(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for _, method := range []string{"list", "members", "remove member"} {
			t.Run(method, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {

					missing := "ListVisibleProjects"
					if method == "members" {
						missing = "ListProjectMembers"
					}
					db, _ := repositoryFixture(allureCtx, missing, nil)
					repo := repository.NewProjectRepository(db)
					user := testutil.UserMother{}.Regular()
					ctx := context.Background()
					var err, want error
					switch method {
					case "list":
						var rows []domain.Project
						rows, err = repo.List(ctx, user.ID, "", 0, 10)
						if len(rows) != 0 {
							allureCtx.Fatalf("rows=%+v", rows)
						}
					case "members":
						var rows []domain.Member
						rows, err = repo.Members(ctx, user.ID, "demo", 0, 10)
						if len(rows) != 0 {
							allureCtx.Fatalf("rows=%+v", rows)
						}
					case "remove member":
						_, err = repo.ChangeMember(ctx, user.ID, "demo", user.ID, 1, true)
						want = domain.ErrMemberNotFound
					}
					if !errors.Is(err, want) {
						allureCtx.Fatalf("err=%v", err)
					}

				})
			})
		}

	})
}

// Техника: переходы транзакции при ошибке аудита.
func TestProjectCreateAuditFailureRollsBack(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		db, tx := repositoryFixture(allureCtx, "", nil)
		failure := errors.New("audit failure")
		old := tx.ExecFunc
		commits, rollbacks := 0, 0
		tx.CommitFunc = func(context.Context) error { commits++; return nil }
		tx.RollbackFunc = func(context.Context) error { rollbacks++; return nil }
		tx.ExecFunc = func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "-- name: AuditProjectChange ") {
				return pgconn.CommandTag{}, failure
			}
			return old(ctx, sql, args...)
		}
		u := testutil.UserMother{}.Regular()
		_, err := repository.NewProjectRepository(db).Create(context.Background(), u.ID, "demo", "")
		if !errors.Is(err, failure) || commits != 0 || rollbacks != 1 {
			allureCtx.Fatalf("err=%v commits=%d rollbacks=%d", err, commits, rollbacks)
		}

	})
}
