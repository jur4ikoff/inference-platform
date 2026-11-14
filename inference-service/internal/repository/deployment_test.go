package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/repository"
	"github.com/jur4ikoff/inference-service/internal/testutil"
	"github.com/jur4ikoff/utils/postgres"
	"go.uber.org/mock/gomock"
)

func deploymentRowValues(d domain.Deployment) []any {
	stamp := pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true}
	return []any{
		pgtype.UUID{Bytes: d.ID, Valid: true}, d.ContainerID,
		pgtype.UUID{Bytes: d.ModelID, Valid: true}, d.Project, d.Username,
		d.Name, d.DeploymentName, string(d.Status),
		[]byte("{\"provider_type\":\"HUGGING_FACE\",\"provider_hugging_face\":{\"repo\":\"org/model\"}}"),
		stamp, stamp,
	}
}

func deploymentStateRowValues(d domain.Deployment) []any {
	return []any{pgtype.UUID{Bytes: d.ID, Valid: true}, d.ContainerID, string(d.Status), d.Project, d.DeploymentName, d.Name}
}

func expectRowQuery(t *testing.T, name string, row pgx.Row) *MockDBTX {
	t.Helper()
	db := NewMockDBTX(gomock.NewController(t))
	db.EXPECT().QueryRow(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, sql string, _ ...any) pgx.Row {
		if !strings.Contains(sql, "-- name: "+name+" ") {
			t.Fatalf("unexpected SQL query for %s: %s", name, sql)
		}
		return row
	})
	return db
}

func expectRowsQuery(t *testing.T, name string, rows pgx.Rows, queryErr error) *MockDBTX {
	t.Helper()
	db := NewMockDBTX(gomock.NewController(t))
	db.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
		if !strings.Contains(sql, "-- name: "+name+" ") {
			t.Fatalf("unexpected SQL query for %s: %s", name, sql)
		}
		return rows, queryErr
	})
	return db
}

func expectExecQuery(t *testing.T, name string, tag pgconn.CommandTag, execErr error) *MockDBTX {
	t.Helper()
	db := NewMockDBTX(gomock.NewController(t))
	db.EXPECT().Exec(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
		if !strings.Contains(sql, "-- name: "+name+" ") {
			t.Fatalf("unexpected SQL query for %s: %s", name, sql)
		}
		return tag, execErr
	})
	return db
}

// Техника: разбиение на классы эквивалентности доступного и недоступного соединения.
func TestDeploymentRepositoryCheckUnit(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := expectRowQuery(t, "CheckConnection", testutil.Row{Values: []any{int32(1)}})
		if err := repository.NewDeploymentRepository(db).Check(t.Context()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("database error", func(t *testing.T) {
		failure := errors.New("database unavailable")
		db := expectRowQuery(t, "CheckConnection", testutil.Row{Err: failure})
		if err := repository.NewDeploymentRepository(db).Check(t.Context()); !errors.Is(err, failure) {
			t.Fatalf("err=%v want=%v", err, failure)
		}
	})
}

// Техника: разбиение на классы эквивалентности активной записи, отсутствия записи и ошибки БД.
func TestDeploymentRepositoryActiveDeploymentExistsUnit(t *testing.T) {
	d := testutil.DeploymentMother{}.Pending()
	for _, tc := range []struct {
		name   string
		exists bool
	}{{"exists", true}, {"absent", false}} {
		t.Run(tc.name, func(t *testing.T) {
			db := expectRowQuery(t, "ActiveDeploymentExists", testutil.Row{Values: []any{tc.exists}})
			exists, err := repository.NewDeploymentRepository(db).ActiveDeploymentExists(t.Context(), d.Project, d.ModelID)
			if err != nil || exists != tc.exists {
				t.Fatalf("exists=%v err=%v", exists, err)
			}
		})
	}
	t.Run("database error", func(t *testing.T) {
		failure := errors.New("database unavailable")
		db := expectRowQuery(t, "ActiveDeploymentExists", testutil.Row{Err: failure})
		exists, err := repository.NewDeploymentRepository(db).ActiveDeploymentExists(t.Context(), d.Project, d.ModelID)
		if exists || !errors.Is(err, failure) {
			t.Fatalf("exists=%v err=%v", exists, err)
		}
	})
}

// Техника: разбиение на классы эквивалентности успешной записи и ошибки БД.
func TestDeploymentRepositoryCreateUnit(t *testing.T) {
	d := testutil.DeploymentMother{}.Pending()
	t.Run("success", func(t *testing.T) {
		db := expectRowQuery(t, "CreateDeployment", testutil.Row{Values: deploymentRowValues(d)})
		got, err := repository.NewDeploymentRepository(db).CreateDeployment(t.Context(), d)
		if err != nil || got.ID != d.ID || got.ContainerID != d.ContainerID || got.Status != d.Status || got.Spec.ProviderHuggingFace == nil || got.Spec.ProviderHuggingFace.Repo != "org/model" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
	t.Run("database error", func(t *testing.T) {
		failure := errors.New("database unavailable")
		db := expectRowQuery(t, "CreateDeployment", testutil.Row{Err: failure})
		got, err := repository.NewDeploymentRepository(db).CreateDeployment(t.Context(), d)
		if got.ID != [16]byte{} || !errors.Is(err, failure) {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
}

// Техника: разбиение на классы эквивалентности найденной, отсутствующей записи и ошибки БД.
func TestDeploymentRepositoryGetDeploymentByIDUnit(t *testing.T) {
	d := testutil.DeploymentMother{}.Pending()
	t.Run("success", func(t *testing.T) {
		db := expectRowQuery(t, "GetDeploymentByID", testutil.Row{Values: deploymentRowValues(d)})
		got, err := repository.NewDeploymentRepository(db).GetDeploymentByID(t.Context(), d.ID)
		if err != nil || got.ID != d.ID || got.Status != d.Status {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		db := expectRowQuery(t, "GetDeploymentByID", testutil.Row{Err: pgx.ErrNoRows})
		_, err := repository.NewDeploymentRepository(db).GetDeploymentByID(t.Context(), d.ID)
		if !errors.Is(err, domain.ErrDeploymentNotFound) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("database error", func(t *testing.T) {
		failure := errors.New("database unavailable")
		db := expectRowQuery(t, "GetDeploymentByID", testutil.Row{Err: failure})
		_, err := repository.NewDeploymentRepository(db).GetDeploymentByID(t.Context(), d.ID)
		if !errors.Is(err, failure) {
			t.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности найденной, отсутствующей записи и ошибки БД.
func TestDeploymentRepositoryGetDeploymentByNameUnit(t *testing.T) {
	d := testutil.DeploymentMother{}.Pending()
	t.Run("success", func(t *testing.T) {
		db := expectRowQuery(t, "GetDeploymentByName", testutil.Row{Values: deploymentRowValues(d)})
		got, err := repository.NewDeploymentRepository(db).GetDeploymentByName(t.Context(), d.Name)
		if err != nil || got.ID != d.ID || got.Name != d.Name {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		db := expectRowQuery(t, "GetDeploymentByName", testutil.Row{Err: pgx.ErrNoRows})
		_, err := repository.NewDeploymentRepository(db).GetDeploymentByName(t.Context(), d.Name)
		if !errors.Is(err, domain.ErrDeploymentNotFound) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("database error", func(t *testing.T) {
		failure := errors.New("database unavailable")
		db := expectRowQuery(t, "GetDeploymentByName", testutil.Row{Err: failure})
		_, err := repository.NewDeploymentRepository(db).GetDeploymentByName(t.Context(), d.Name)
		if !errors.Is(err, failure) {
			t.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности непустого и пустого списка, ошибки БД; граничное значение limit.
func TestDeploymentRepositoryListDeploymentsUnit(t *testing.T) {
	d := testutil.DeploymentMother{}.Pending()
	t.Run("success", func(t *testing.T) {
		rows := &testutil.Rows{Data: [][]any{deploymentRowValues(d)}}
		db := expectRowsQuery(t, "ListDeployments", rows, nil)
		got, err := repository.NewDeploymentRepository(db).ListDeployments(t.Context(), 0, 10)
		if err != nil || len(got) != 1 || got[0].ID != d.ID || !rows.Closed {
			t.Fatalf("got=%+v err=%v closed=%v", got, err, rows.Closed)
		}
	})
	t.Run("empty", func(t *testing.T) {
		rows := &testutil.Rows{}
		db := expectRowsQuery(t, "ListDeployments", rows, nil)
		got, err := repository.NewDeploymentRepository(db).ListDeployments(t.Context(), 0, 10)
		if err != nil || len(got) != 0 || !rows.Closed {
			t.Fatalf("got=%+v err=%v closed=%v", got, err, rows.Closed)
		}
	})
	t.Run("database error", func(t *testing.T) {
		failure := errors.New("database unavailable")
		db := expectRowsQuery(t, "ListDeployments", nil, failure)
		got, err := repository.NewDeploymentRepository(db).ListDeployments(t.Context(), 0, 10)
		if got != nil || !errors.Is(err, failure) {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
	t.Run("invalid limit", func(t *testing.T) {
		_, err := repository.NewDeploymentRepository(nil).ListDeployments(t.Context(), 0, 0)
		if !errors.Is(err, domain.ErrInvalidPagination) {
			t.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности непустой истории, пустого списка и ошибки БД; граничное значение offset.
func TestDeploymentRepositoryListAllDeploymentsUnit(t *testing.T) {
	d := testutil.DeploymentMother{}.Stopped()
	t.Run("success", func(t *testing.T) {
		rows := &testutil.Rows{Data: [][]any{deploymentRowValues(d)}}
		db := expectRowsQuery(t, "ListAllDeployments", rows, nil)
		got, err := repository.NewDeploymentRepository(db).ListAllDeployments(t.Context(), 0, 10)
		if err != nil || len(got) != 1 || got[0].ID != d.ID || got[0].Status != domain.StatusStopped || !rows.Closed {
			t.Fatalf("got=%+v err=%v closed=%v", got, err, rows.Closed)
		}
	})
	t.Run("empty", func(t *testing.T) {
		rows := &testutil.Rows{}
		db := expectRowsQuery(t, "ListAllDeployments", rows, nil)
		got, err := repository.NewDeploymentRepository(db).ListAllDeployments(t.Context(), 0, 10)
		if err != nil || len(got) != 0 || !rows.Closed {
			t.Fatalf("got=%+v err=%v closed=%v", got, err, rows.Closed)
		}
	})
	t.Run("database error", func(t *testing.T) {
		failure := errors.New("database unavailable")
		db := expectRowsQuery(t, "ListAllDeployments", nil, failure)
		got, err := repository.NewDeploymentRepository(db).ListAllDeployments(t.Context(), 0, 10)
		if got != nil || !errors.Is(err, failure) {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
	t.Run("negative offset", func(t *testing.T) {
		_, err := repository.NewDeploymentRepository(nil).ListAllDeployments(t.Context(), -1, 10)
		if !errors.Is(err, domain.ErrInvalidPagination) {
			t.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности непустого и пустого списка состояний, ошибки БД.
func TestDeploymentRepositoryListDeploymentStatesUnit(t *testing.T) {
	d := testutil.DeploymentMother{}.Running()
	t.Run("success", func(t *testing.T) {
		rows := &testutil.Rows{Data: [][]any{deploymentStateRowValues(d)}}
		db := expectRowsQuery(t, "ListDeploymentStates", rows, nil)
		got, err := repository.NewDeploymentRepository(db).ListDeploymentStates(t.Context())
		if err != nil || len(got) != 1 || got[0].ID != d.ID || got[0].Status != domain.StatusRunning || !rows.Closed {
			t.Fatalf("got=%+v err=%v closed=%v", got, err, rows.Closed)
		}
	})
	t.Run("empty", func(t *testing.T) {
		rows := &testutil.Rows{}
		db := expectRowsQuery(t, "ListDeploymentStates", rows, nil)
		got, err := repository.NewDeploymentRepository(db).ListDeploymentStates(t.Context())
		if err != nil || len(got) != 0 || !rows.Closed {
			t.Fatalf("got=%+v err=%v closed=%v", got, err, rows.Closed)
		}
	})
	t.Run("database error", func(t *testing.T) {
		failure := errors.New("database unavailable")
		db := expectRowsQuery(t, "ListDeploymentStates", nil, failure)
		got, err := repository.NewDeploymentRepository(db).ListDeploymentStates(t.Context())
		if got != nil || !errors.Is(err, failure) {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
}

// Техника: разбиение на классы эквивалентности найденного, отсутствующего состояния и ошибки БД.
func TestDeploymentRepositoryGetDeploymentStateUnit(t *testing.T) {
	d := testutil.DeploymentMother{}.Running()
	t.Run("success", func(t *testing.T) {
		db := expectRowQuery(t, "GetDeploymentState", testutil.Row{Values: deploymentStateRowValues(d)})
		got, err := repository.NewDeploymentRepository(db).GetDeploymentState(t.Context(), d.ContainerID)
		if err != nil || got.ID != d.ID || got.Status != domain.StatusRunning {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		db := expectRowQuery(t, "GetDeploymentState", testutil.Row{Err: pgx.ErrNoRows})
		_, err := repository.NewDeploymentRepository(db).GetDeploymentState(t.Context(), d.ContainerID)
		if !errors.Is(err, domain.ErrDeploymentNotFound) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("database error", func(t *testing.T) {
		failure := errors.New("database unavailable")
		db := expectRowQuery(t, "GetDeploymentState", testutil.Row{Err: failure})
		_, err := repository.NewDeploymentRepository(db).GetDeploymentState(t.Context(), d.ContainerID)
		if !errors.Is(err, failure) {
			t.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; одна или ноль обновлённых строк и ошибка БД.
func TestDeploymentRepositoryUpdateDeploymentStatusUnit(t *testing.T) {
	d := testutil.DeploymentMother{}.Pending()
	previous := domain.DeploymentState{ID: d.ID, ContainerID: d.ContainerID, Status: domain.StatusPending}
	t.Run("updated", func(t *testing.T) {
		db := expectExecQuery(t, "UpdateDeploymentStatus", pgconn.NewCommandTag("UPDATE 1"), nil)
		updated, err := repository.NewDeploymentRepository(db).UpdateDeploymentStatus(t.Context(), previous, domain.StatusStarted)
		if err != nil || !updated {
			t.Fatalf("updated=%v err=%v", updated, err)
		}
	})
	t.Run("stale state", func(t *testing.T) {
		db := expectExecQuery(t, "UpdateDeploymentStatus", pgconn.NewCommandTag("UPDATE 0"), nil)
		updated, err := repository.NewDeploymentRepository(db).UpdateDeploymentStatus(t.Context(), previous, domain.StatusStarted)
		if err != nil || updated {
			t.Fatalf("updated=%v err=%v", updated, err)
		}
	})
	t.Run("database error", func(t *testing.T) {
		failure := errors.New("database unavailable")
		db := expectExecQuery(t, "UpdateDeploymentStatus", pgconn.CommandTag{}, failure)
		updated, err := repository.NewDeploymentRepository(db).UpdateDeploymentStatus(t.Context(), previous, domain.StatusStarted)
		if updated || !errors.Is(err, failure) {
			t.Fatalf("updated=%v err=%v", updated, err)
		}
	})
}

// Техника: разбиение на классы эквивалентности недопустимых состояний при создании.
func TestDeploymentRepositoryCreateRejectsInvalidStatus(t *testing.T) {
	// No DB is supplied: validation must finish before executing any SQL.
	repo := repository.NewDeploymentRepository(nil)
	for _, status := range []domain.Status{"", "INVALID"} {
		if _, err := repo.CreateDeployment(t.Context(), domain.Deployment{Status: status}); !errors.Is(err, domain.ErrInvalidStatus) {
			t.Fatalf("expected invalid create status, got %v", err)
		}
	}
}

// Техника: разбиение на классы эквивалентности недопустимого нового и предыдущего состояния.
func TestDeploymentRepositoryUpdateRejectsInvalidStatus(t *testing.T) {
	repo := repository.NewDeploymentRepository(nil)
	for _, tc := range []struct {
		name           string
		previous, next domain.Status
	}{
		{"new status is empty", domain.StatusPending, ""},
		{"new status is unknown", domain.StatusPending, "INVALID"},
		{"previous status is empty", "", domain.StatusStarted},
		{"previous status is unknown", "INVALID", domain.StatusStarted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.UpdateDeploymentStatus(t.Context(), domain.DeploymentState{Status: tc.previous}, tc.next)
			if !errors.Is(err, domain.ErrInvalidStatus) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// Техника: разбиение на классы эквивалентности; случай конфликта активного запуска.
func TestDeploymentCreateDuplicate(t *testing.T) {
	db := NewMockDBTX(gomock.NewController(t))
	db.EXPECT().QueryRow(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
			return testutil.Row{Err: &pgconn.PgError{Code: "23505", ConstraintName: "deployments_project_model_id_key"}}
		})
	_, err := repository.NewDeploymentRepository(db).CreateDeployment(context.Background(), testutil.DeploymentMother{}.Pending())
	if !errors.Is(err, domain.ErrDeploymentAlreadyExists) {
		t.Fatalf("err=%v", err)
	}
}

// Техника: разбиение на классы эквивалентности ошибок при чтении набора строк.
func TestDeploymentListReadErrors(t *testing.T) {
	for _, scenario := range []string{"scan", "iteration"} {
		t.Run(scenario, func(t *testing.T) {
			failure := errors.New("read failed")
			rows := &testutil.Rows{Data: [][]any{{}}}
			if scenario == "scan" {
				rows.ScanErr = failure
			} else {
				rows.Data = nil
				rows.IterErr = failure
			}
			db := NewMockDBTX(gomock.NewController(t))
			db.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).Return(rows, nil)
			_, err := repository.NewDeploymentRepository(db).ListAllDeployments(context.Background(), 0, 10)
			if !errors.Is(err, failure) || !rows.Closed {
				t.Fatalf("err=%v closed=%v", err, rows.Closed)
			}
		})
	}
}

func integrationDeploymentRepository(t *testing.T) (context.Context, pgx.Tx, *repository.DeploymentRepository) {
	t.Helper()
	databaseURL := os.Getenv("INFERENCE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set INFERENCE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	pool, err := postgres.NewPool(ctx, databaseURL, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	// Temporary tables shadow application tables on this transaction's connection.
	for _, name := range []string{"000006_create_deployments_table.sql", "000007_deployment_created_at.sql", "000008_deployment_updated_at.sql"} {
		migration, err := os.ReadFile(filepath.Join("..", "..", "..", "infra", "postgres", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		schema := strings.Replace(string(migration), "CREATE TABLE deployments", "CREATE TEMP TABLE deployments", 1)
		if _, err := tx.Exec(ctx, schema); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `
  CREATE FUNCTION pg_temp.normalize_deployment_username() RETURNS trigger
  LANGUAGE plpgsql AS $$
  BEGIN
   NEW.username := upper(NEW.username);
   RETURN NEW;
  END;
  $$;
  CREATE TRIGGER normalize_deployment_username
  BEFORE INSERT ON deployments FOR EACH ROW
  EXECUTE FUNCTION pg_temp.normalize_deployment_username();
 `); err != nil {
		t.Fatal(err)
	}
	return ctx, tx, repository.NewDeploymentRepository(tx)
}

func pendingDeployment() domain.Deployment {
	return domain.Deployment{
		ID: uuid.New(), ContainerID: uuid.NewString(), ModelID: uuid.New(),
		Project: "project-1", Username: "alice", Name: "qwen-small-abcde", DeploymentName: "qwen-small", Status: domain.StatusPending,
		Spec: domain.DeploymentSpec{ProviderType: domain.ModelProviderTypeHuggingFace, ProviderHuggingFace: &domain.HuggingFaceProvider{Repo: "Qwen/model"}},
	}
}

func createDeployment(t *testing.T, ctx context.Context, db *repository.DeploymentRepository, deployment domain.Deployment) domain.Deployment {
	t.Helper()
	saved, err := db.CreateDeployment(ctx, deployment)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

// Техника: разбиение на классы эквивалентности; корректная запись с вычисляемыми БД значениями.
func TestDeploymentIntegrationCreateDeploymentPersistsReturnedValues(t *testing.T) {
	ctx, tx, db := integrationDeploymentRepository(t)
	input := pendingDeployment()
	saved := createDeployment(t, ctx, db, input)
	input.Username = "ALICE"
	if saved.StartedAt == nil || saved.UpdatedAt == nil {
		t.Fatal("timestamps are missing")
	}
	input.StartedAt, input.UpdatedAt = saved.StartedAt, saved.UpdatedAt
	if !reflect.DeepEqual(saved, input) {
		t.Fatalf("returned deployment: got %+v, want %+v", saved, input)
	}
	var actual domain.Deployment
	var spec []byte
	var specType, providerType, repo string
	err := tx.QueryRow(ctx, `SELECT id, container_id, model_id, project, username, name, deployment_name, status, spec,
  created_at, updated_at, pg_typeof(spec)::text, spec->>'provider_type', spec->'provider_hugging_face'->>'repo'
  FROM deployments WHERE id = $1`, input.ID).Scan(
		&actual.ID, &actual.ContainerID, &actual.ModelID, &actual.Project, &actual.Username,
		&actual.Name, &actual.DeploymentName, &actual.Status, &spec, &actual.StartedAt, &actual.UpdatedAt,
		&specType, &providerType, &repo,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(spec, &actual.Spec); err != nil {
		t.Fatal(err)
	}
	if actual.StartedAt != nil {
		value := actual.StartedAt.UTC()
		actual.StartedAt = &value
	}
	if actual.UpdatedAt != nil {
		value := actual.UpdatedAt.UTC()
		actual.UpdatedAt = &value
	}
	if !reflect.DeepEqual(actual, input) || specType != "jsonb" || providerType != "HUGGING_FACE" || repo != "Qwen/model" {
		t.Fatalf("persisted deployment=%+v, spec type=%s, provider=%s, repo=%s", actual, specType, providerType, repo)
	}
}

// Техника: разбиение на классы эквивалентности активного и завершённого запуска.
func TestDeploymentIntegrationActiveDeploymentExistsTracksStatus(t *testing.T) {
	ctx, tx, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	exists, err := db.ActiveDeploymentExists(ctx, deployment.Project, deployment.ModelID)
	if err != nil || exists {
		t.Fatalf("before create: exists=%v err=%v", exists, err)
	}
	createDeployment(t, ctx, db, deployment)
	exists, err = db.ActiveDeploymentExists(ctx, deployment.Project, deployment.ModelID)
	if err != nil || !exists {
		t.Fatalf("pending: exists=%v err=%v", exists, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE deployments SET status = 'STOPPED' WHERE id = $1`, deployment.ID); err != nil {
		t.Fatal(err)
	}
	exists, err = db.ActiveDeploymentExists(ctx, deployment.Project, deployment.ModelID)
	if err != nil || exists {
		t.Fatalf("stopped: exists=%v err=%v", exists, err)
	}
}

// Техника: разбиение на классы эквивалентности проекта и модели.
func TestDeploymentIntegrationActiveDeploymentExistsScopesByProjectAndModel(t *testing.T) {
	ctx, _, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	createDeployment(t, ctx, db, deployment)
	for _, tc := range []struct {
		project string
		modelID uuid.UUID
	}{
		{"another-project", deployment.ModelID}, {deployment.Project, uuid.New()},
	} {
		exists, err := db.ActiveDeploymentExists(ctx, tc.project, tc.modelID)
		if err != nil || exists {
			t.Fatalf("project=%s model=%s exists=%v err=%v", tc.project, tc.modelID, exists, err)
		}
	}
}

// Техника: разбиение на классы эквивалентности; случай существующего состояния deployment.
func TestDeploymentIntegrationGetDeploymentStateReadsPending(t *testing.T) {
	ctx, _, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	createDeployment(t, ctx, db, deployment)
	state, err := db.GetDeploymentState(ctx, deployment.ContainerID)
	if err != nil || state.ID != deployment.ID || state.Status != domain.StatusPending {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}

// Техника: разбиение на классы эквивалентности; случай отсутствующего контейнера.
func TestDeploymentIntegrationGetDeploymentStateReportsMissingContainer(t *testing.T) {
	ctx, _, db := integrationDeploymentRepository(t)
	_, err := db.GetDeploymentState(ctx, "missing-container")
	if !errors.Is(err, domain.ErrDeploymentNotFound) {
		t.Fatalf("err=%v", err)
	}
}

// Техника: разбиение на классы эквивалентности; случай некорректного статуса, сохранённого в БД.
func TestDeploymentIntegrationGetDeploymentStateRejectsInvalidStoredStatus(t *testing.T) {
	ctx, tx, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	createDeployment(t, ctx, db, deployment)
	if _, err := tx.Exec(ctx, `UPDATE deployments SET status = 'CUSTOM_STATUS' WHERE id = $1`, deployment.ID); err != nil {
		t.Fatal(err)
	}
	_, err := db.GetDeploymentState(ctx, deployment.ContainerID)
	if !errors.Is(err, domain.ErrInvalidStatus) {
		t.Fatalf("err=%v", err)
	}
}

// Техника: разбиение на классы эквивалентности; случай существующего состояния при чтении списка.
func TestDeploymentIntegrationListDeploymentStatesReturnsPending(t *testing.T) {
	ctx, _, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	createDeployment(t, ctx, db, deployment)
	states, err := db.ListDeploymentStates(ctx)
	if err != nil || len(states) != 1 || states[0].ID != deployment.ID || states[0].Status != domain.StatusPending {
		t.Fatalf("states=%+v err=%v", states, err)
	}
}

// Техника: разбиение на классы эквивалентности; случай некорректного статуса в списке состояний.
func TestDeploymentIntegrationListDeploymentStatesRejectsInvalidStoredStatus(t *testing.T) {
	ctx, tx, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	createDeployment(t, ctx, db, deployment)
	if _, err := tx.Exec(ctx, `UPDATE deployments SET status = 'CUSTOM_STATUS' WHERE id = $1`, deployment.ID); err != nil {
		t.Fatal(err)
	}
	_, err := db.ListDeploymentStates(ctx)
	if !errors.Is(err, domain.ErrInvalidStatus) {
		t.Fatalf("err=%v", err)
	}
}

// Техника: тестирование переходов состояний; переход PENDING → STARTED.
func TestDeploymentIntegrationUpdateDeploymentStatusPersistsTransition(t *testing.T) {
	ctx, _, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	createDeployment(t, ctx, db, deployment)
	previous, err := db.GetDeploymentState(ctx, deployment.ContainerID)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := db.UpdateDeploymentStatus(ctx, previous, domain.StatusStarted)
	if err != nil || !updated {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
	current, err := db.GetDeploymentState(ctx, deployment.ContainerID)
	if err != nil || current.Status != domain.StatusStarted {
		t.Fatalf("state=%+v err=%v", current, err)
	}
}

// Техника: тестирование переходов состояний при устаревшем снимке.
func TestDeploymentIntegrationUpdateDeploymentStatusIgnoresStaleState(t *testing.T) {
	ctx, tx, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	createDeployment(t, ctx, db, deployment)
	previous, err := db.GetDeploymentState(ctx, deployment.ContainerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE deployments SET status = 'STARTED' WHERE id = $1`, deployment.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := db.UpdateDeploymentStatus(ctx, previous, domain.StatusStopped)
	if err != nil || updated {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
}

// Техника: разбиение на классы эквивалентности; случай несовпадающего идентификатора контейнера.
func TestDeploymentIntegrationUpdateDeploymentStatusIgnoresReplacedContainer(t *testing.T) {
	ctx, _, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	createDeployment(t, ctx, db, deployment)
	previous, err := db.GetDeploymentState(ctx, deployment.ContainerID)
	if err != nil {
		t.Fatal(err)
	}
	previous.ContainerID = "replaced-container-id"
	updated, err := db.UpdateDeploymentStatus(ctx, previous, domain.StatusStarted)
	if err != nil || updated {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
}

// Техника: тестирование переходов состояний; переход от завершённого запуска к повторному запуску.
func TestDeploymentIntegrationCreateDeploymentAllowsRestartAfterStop(t *testing.T) {
	ctx, tx, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	createDeployment(t, ctx, db, deployment)
	if _, err := tx.Exec(ctx, `UPDATE deployments SET status = 'STOPPED' WHERE id = $1`, deployment.ID); err != nil {
		t.Fatal(err)
	}
	replacement := deployment
	replacement.ID, replacement.ContainerID, replacement.Name = uuid.New(), uuid.NewString(), "replacement-qwen-name"
	if _, err := db.CreateDeployment(ctx, replacement); err != nil {
		t.Fatalf("restart failed: %v", err)
	}
}

// Техника: разбиение на классы эквивалентности; случай независимых проектов.
func TestDeploymentIntegrationCreateDeploymentAllowsSameModelInOtherProject(t *testing.T) {
	ctx, _, db := integrationDeploymentRepository(t)
	deployment := pendingDeployment()
	createDeployment(t, ctx, db, deployment)
	another := deployment
	another.ID, another.Project, another.ContainerID, another.Name = uuid.New(), "another-project", uuid.NewString(), "other-qwen-name"
	if _, err := db.CreateDeployment(ctx, another); err != nil {
		t.Fatalf("other project: %v", err)
	}
}

// Техника: разбиение на классы эквивалентности ограничений уникальности модели и имени.
func TestDeploymentIntegrationCreateDeploymentRejectsActiveDuplicates(t *testing.T) {
	for _, tc := range []struct {
		name, constraint string
		change           func(*domain.Deployment)
	}{
		{"model", "deployments_project_model_id_key", func(d *domain.Deployment) { d.DeploymentName = "other-deployment" }},
		{"name", "deployments_project_deployment_name_key", func(d *domain.Deployment) { d.ModelID = uuid.New() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, db := integrationDeploymentRepository(t)
			deployment := pendingDeployment()
			createDeployment(t, ctx, db, deployment)
			duplicate := deployment
			duplicate.ID, duplicate.ContainerID, duplicate.Name = uuid.New(), uuid.NewString(), "another-random-name"
			tc.change(&duplicate)
			_, err := db.CreateDeployment(ctx, duplicate)
			var pgErr *pgconn.PgError
			if !errors.Is(err, domain.ErrDeploymentAlreadyExists) || !errors.As(err, &pgErr) || pgErr.ConstraintName != tc.constraint {
				t.Fatalf("constraint=%s err=%v", tc.constraint, err)
			}
		})
	}
}

// Техника: тестирование переходов состояний; завершённый запуск и повторный запуск.
func TestDeploymentIntegrationListAllDeploymentsReturnsNewestFirst(t *testing.T) {
	ctx, tx, db := integrationDeploymentRepository(t)
	first := pendingDeployment()
	createDeployment(t, ctx, db, first)
	if _, err := tx.Exec(ctx, `UPDATE deployments SET status = 'STOPPED' WHERE id = $1`, first.ID); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID, second.ContainerID, second.Name = uuid.New(), uuid.NewString(), "second-qwen-name"
	createDeployment(t, ctx, db, second)
	history, err := db.ListAllDeployments(ctx, 0, 10)
	if err != nil || len(history) != 2 || history[0].ID != second.ID || history[1].ID != first.ID {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

// Техника: разбиение на классы эквивалентности; случай доступной БД.
func TestDeploymentIntegrationRepositoryCheck(t *testing.T) {
	databaseURL := os.Getenv("INFERENCE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set INFERENCE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := postgres.NewPool(ctx, databaseURL, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	repo := repository.NewDeploymentRepository(pool)
	if err := repo.Check(ctx); err != nil {
		t.Fatal(err)
	}
}

// Техника: разбиение на классы эквивалентности; случай отменённого контекста запроса к БД.
func TestDeploymentIntegrationRepositoryCheckCanceledContext(t *testing.T) {
	databaseURL := os.Getenv("INFERENCE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set INFERENCE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	pool, err := postgres.NewPool(ctx, databaseURL, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	canceledCtx, cancelCheck := context.WithCancel(ctx)
	cancelCheck()
	if err := repository.NewDeploymentRepository(pool).Check(canceledCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
