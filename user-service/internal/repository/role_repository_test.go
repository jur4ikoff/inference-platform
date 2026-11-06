package repository_test

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/repository"
	"github.com/jur4ikoff/user-service/internal/testutil"
	"strings"
	"testing"
)

// Техника: классы эквивалентности успешного запроса и ошибки БД.
func TestRoleRepositoryUnit(t *testing.T) {
	for _, method := range []string{"create", "get", "list", "update", "delete"} {
		for _, scenario := range []string{"success", "database error"} {
			t.Run(method+"/"+scenario, func(t *testing.T) {
				failure := errors.New("database error")
				var injected error
				if scenario == "database error" {
					injected = failure
				}
				db, _ := repositoryFixture(t, "", injected)
				r := repository.NewRoleRepository(db)
				u := testutil.UserMother{}.Regular()
				role := testutil.RoleMother{}.Developer()
				name := "updated"
				ctx := context.Background()
				var err error
				switch method {
				case "create":
					_, err = r.Create(ctx, u.ID, role)
				case "get":
					_, err = r.Get(ctx, role.ID)
				case "list":
					_, err = r.List(ctx, "", 0, 10)
				case "update":
					_, err = r.Update(ctx, u.ID, role.ID, domain.RoleUpdate{Name: &name})
				case "delete":
					err = r.Delete(ctx, u.ID, role.ID)
				}
				if scenario == "success" && err != nil {
					t.Fatalf("err=%v", err)
				}
				if scenario == "database error" && !errors.Is(err, failure) {
					t.Fatalf("err=%v", err)
				}
			})
		}
	}
}

// Техника: класс отсутствующей роли.
func TestRoleRepositoryMissing(t *testing.T) {
	for _, method := range []string{"get", "update", "delete"} {
		t.Run(method, func(t *testing.T) {
			missing := "LockRole"
			if method == "get" {
				missing = "GetRole"
			}
			db, _ := repositoryFixture(t, missing, nil)
			r := repository.NewRoleRepository(db)
			u := testutil.UserMother{}.Regular()
			ctx := context.Background()
			var err error
			switch method {
			case "get":
				_, err = r.Get(ctx, 1)
			case "update":
				name := "updated"
				_, err = r.Update(ctx, u.ID, 1, domain.RoleUpdate{Name: &name})
			case "delete":
				err = r.Delete(ctx, u.ID, 1)
			}
			if !errors.Is(err, domain.ErrRoleNotFound) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// Техника: класс существующей роли и обработка результата.
func TestRoleRepositoryMapsResult(t *testing.T) {
	db, _ := repositoryFixture(t, "", nil)
	r := repository.NewRoleRepository(db)
	role, err := r.Get(context.Background(), 1)
	if err != nil || role.ID != 1 || role.Name != "developer" || !role.CanReadModel {
		t.Fatalf("role=%+v err=%v", role, err)
	}
}

// Техника: класс конфликта уникального имени роли.
func TestRoleRepositoryDuplicate(t *testing.T) {
	db, tx := repositoryFixture(t, "", nil)
	original := tx.QueryRowFunc
	tx.QueryRowFunc = func(ctx context.Context, sql string, args ...interface{}) pgx.Row {
		if strings.Contains(sql, "-- name: CreateRole ") {
			return testutil.Row{Err: &pgconn.PgError{Code: "23505", ConstraintName: "roles_name_key"}}
		}
		return original(ctx, sql, args...)
	}
	user := testutil.UserMother{}.Regular()
	_, err := repository.NewRoleRepository(db).Create(context.Background(), user.ID, testutil.RoleMother{}.Developer())
	if !errors.Is(err, domain.ErrRoleAlreadyExists) {
		t.Fatalf("err=%v", err)
	}
}

// Техника: класс пустого результата списка.
func TestRoleRepositoryEmptyList(t *testing.T) {
	db, _ := repositoryFixture(t, "ListRoles", nil)
	roles, err := repository.NewRoleRepository(db).List(context.Background(), "", 0, 10)
	if err != nil || len(roles) != 0 {
		t.Fatalf("roles=%+v err=%v", roles, err)
	}
}

// Техника: переходы транзакции при ошибке аудита.
func TestRoleCreateAuditFailureRollsBack(t *testing.T) {
	db, tx := repositoryFixture(t, "", nil)
	failure := errors.New("audit failure")
	old := tx.ExecFunc
	commits, rollbacks := 0, 0
	tx.CommitFunc = func(context.Context) error { commits++; return nil }
	tx.RollbackFunc = func(context.Context) error { rollbacks++; return nil }
	tx.ExecFunc = func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
		if strings.Contains(sql, "-- name: AuditRoleChange ") {
			return pgconn.CommandTag{}, failure
		}
		return old(ctx, sql, args...)
	}
	u := testutil.UserMother{}.Regular()
	_, err := repository.NewRoleRepository(db).Create(context.Background(), u.ID, testutil.RoleMother{}.Developer())
	if !errors.Is(err, failure) || commits != 0 || rollbacks != 1 {
		t.Fatalf("err=%v commits=%d rollbacks=%d", err, commits, rollbacks)
	}
}
