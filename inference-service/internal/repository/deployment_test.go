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
)

// Техника: классы эквивалентности доступной и недоступной БД.
func TestDeploymentRepositoryCheck(t *testing.T) {
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
	canceledCtx, cancelCheck := context.WithCancel(ctx)
	cancelCheck()
	if err := repo.Check(canceledCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

// Техника: классы эквивалентности допустимых и недопустимых состояний.
func TestDeploymentRepositoryRejectsInvalidStatusesBeforeDatabase(t *testing.T) {
	// No DB is supplied: validation must finish before executing any SQL.
	repo := repository.NewDeploymentRepository(nil)
	for _, status := range []domain.Status{"", "INVALID"} {
		if _, err := repo.CreateDeployment(t.Context(), domain.Deployment{Status: status}); !errors.Is(err, domain.ErrInvalidStatus) {
			t.Fatalf("expected invalid create status, got %v", err)
		}
		if _, err := repo.UpdateDeploymentStatus(t.Context(), domain.DeploymentState{Status: domain.StatusPending}, status); !errors.Is(err, domain.ErrInvalidStatus) {
			t.Fatalf("expected invalid new status, got %v", err)
		}
		if _, err := repo.UpdateDeploymentStatus(t.Context(), domain.DeploymentState{Status: status}, domain.StatusStarted); !errors.Is(err, domain.ErrInvalidStatus) {
			t.Fatalf("expected invalid previous status, got %v", err)
		}
	}
}

// Техника: переходы состояний запуска и классы эквивалентности ограничений уникальности.
func TestCreateDeployment(t *testing.T) {
	databaseURL := os.Getenv("INFERENCE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set INFERENCE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.NewPool(ctx, databaseURL, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())

	// Temporary tables shadow existing tables only on this connection.
	// Run the real migration without changing the application's schema or data.
	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "infra", "postgres", "migrations", "000006_create_deployments_table.sql"))
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Replace(string(migration), "CREATE TABLE deployments", "CREATE TEMP TABLE deployments", 1)
	if _, err := tx.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	migration, err = os.ReadFile(filepath.Join("..", "..", "..", "infra", "postgres", "migrations", "000007_deployment_created_at.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	migration, err = os.ReadFile(filepath.Join("..", "..", "..", "infra", "postgres", "migrations", "000008_deployment_updated_at.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}

	// A database-side change distinguishes the returned row from the input.
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
	database := repository.NewDeploymentRepository(tx)
	expected := domain.Deployment{
		ID: uuid.New(), ContainerID: "docker-container-id", ModelID: uuid.New(),
		Project: "project-1", Username: "alice", Name: "qwen-small-abcde", DeploymentName: "qwen-small", Status: domain.StatusPending,
		Spec: domain.DeploymentSpec{
			ProviderType: domain.ModelProviderTypeHuggingFace,
			ProviderHuggingFace: &domain.HuggingFaceProvider{
				Repo: "Qwen/model",
			},
		},
	}
	if exists, err := database.ActiveDeploymentExists(ctx, expected.Project, expected.ModelID); err != nil || exists {
		t.Fatalf("new deployment unexpectedly exists: exists=%v error=%v", exists, err)
	}
	saved, err := database.CreateDeployment(ctx, expected)
	if err != nil {
		t.Fatal(err)
	}
	expected.Username = "ALICE"
	if saved.StartedAt == nil || saved.UpdatedAt == nil {
		t.Fatal("new deployment must have timestamps")
	}
	expected.StartedAt, expected.UpdatedAt = saved.StartedAt, saved.UpdatedAt
	if !reflect.DeepEqual(saved, expected) {
		t.Fatalf("returned deployment differs: got %+v, want %+v", saved, expected)
	}
	var actual domain.Deployment
	var spec []byte
	var specType, providerType, repo string
	if err := tx.QueryRow(ctx, `
		SELECT id, container_id, model_id, project, username, name, deployment_name, status, spec,
		       created_at, updated_at, pg_typeof(spec)::text, spec->>'provider_type', spec->'provider_hugging_face'->>'repo'
		FROM deployments WHERE id = $1`, expected.ID).Scan(
		&actual.ID, &actual.ContainerID, &actual.ModelID, &actual.Project, &actual.Username,
		&actual.Name, &actual.DeploymentName, &actual.Status, &spec, &actual.StartedAt, &actual.UpdatedAt, &specType, &providerType, &repo,
	); err != nil {
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
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("saved deployment differs: got %+v, want %+v", actual, expected)
	}
	if specType != "jsonb" || providerType != "HUGGING_FACE" || repo != "Qwen/model" {
		t.Fatalf("spec is not queryable jsonb: type=%s provider=%s repo=%s", specType, providerType, repo)
	}
	states, err := database.ListDeploymentStates(ctx)
	if err != nil || len(states) != 1 || states[0].ID != expected.ID || states[0].Status != domain.StatusPending {
		t.Fatalf("incorrect deployment states: %+v, %v", states, err)
	}
	previous, err := database.GetDeploymentState(ctx, expected.ContainerID)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := database.UpdateDeploymentStatus(ctx, previous, domain.StatusStarted)
	if err != nil || !updated {
		t.Fatalf("failed to mark container started: updated=%v error=%v", updated, err)
	}
	updated, err = database.UpdateDeploymentStatus(ctx, previous, domain.StatusStopped)
	if err != nil || updated {
		t.Fatalf("stale status update must be ignored: updated=%v error=%v", updated, err)
	}
	for _, next := range []domain.Status{domain.StatusStopping, domain.StatusStopped} {
		previous, err = database.GetDeploymentState(ctx, expected.ContainerID)
		if err != nil {
			t.Fatal(err)
		}
		updated, err = database.UpdateDeploymentStatus(ctx, previous, next)
		if err != nil || !updated {
			t.Fatalf("failed to update status to %s: updated=%v error=%v", next, updated, err)
		}
	}
	previous, err = database.GetDeploymentState(ctx, expected.ContainerID)
	if err != nil || previous.Status != domain.StatusStopped {
		t.Fatalf("expected STOPPED, got %+v error=%v", previous, err)
	}
	previous.ContainerID = "replaced-container-id"
	updated, err = database.UpdateDeploymentStatus(ctx, previous, domain.StatusStarted)
	if err != nil || updated {
		t.Fatalf("update for a different container must be ignored: updated=%v error=%v", updated, err)
	}
	if _, err := database.GetDeploymentState(ctx, "missing-container"); !errors.Is(err, domain.ErrDeploymentNotFound) {
		t.Fatalf("expected deployment not found, got %v", err)
	}
	var statusType string
	if err := tx.QueryRow(ctx, `SELECT pg_typeof(status)::text FROM deployments WHERE id = $1`, expected.ID).Scan(&statusType); err != nil || statusType != "text" {
		t.Fatalf("status must be stored as text: type=%s error=%v", statusType, err)
	}
	// PostgreSQL accepts arbitrary text; Go validates it at the repository boundary.
	if _, err := tx.Exec(ctx, `UPDATE deployments SET status = 'CUSTOM_STATUS' WHERE id = $1`, expected.ID); err != nil {
		t.Fatalf("database must not restrict status values: %v", err)
	}
	if _, err := database.GetDeploymentState(ctx, expected.ContainerID); !errors.Is(err, domain.ErrInvalidStatus) {
		t.Fatalf("expected Go validation of the stored status, got %v", err)
	}
	if _, err := database.ListDeploymentStates(ctx); !errors.Is(err, domain.ErrInvalidStatus) {
		t.Fatalf("expected Go validation when listing statuses, got %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE deployments SET status = 'STOPPED' WHERE id = $1`, expected.ID); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		project string
		modelID uuid.UUID
		want    bool
	}{
		{expected.Project, expected.ModelID, false},
		{"another-project", expected.ModelID, false},
		{expected.Project, uuid.New(), false},
	} {
		exists, err := database.ActiveDeploymentExists(ctx, test.project, test.modelID)
		if err != nil || exists != test.want {
			t.Fatalf("wrong existence result for %s/%s: exists=%v error=%v", test.project, test.modelID, exists, err)
		}
	}
	replacement := expected
	replacement.ID = uuid.New()
	replacement.ContainerID, replacement.Name = "replacement-docker-id", "replacement-qwen-name"
	if _, err := database.CreateDeployment(ctx, replacement); err != nil {
		t.Fatalf("stopped deployment must allow a new deployment of the same model and name: %v", err)
	}
	if exists, err := database.ActiveDeploymentExists(ctx, expected.Project, expected.ModelID); err != nil || !exists {
		t.Fatalf("new pending deployment must block another start: exists=%v error=%v", exists, err)
	}
	if history, err := database.ListAllDeployments(ctx, 0, 10); err != nil || len(history) != 2 || history[0].ID != replacement.ID || history[1].ID != expected.ID {
		t.Fatalf("history must preserve attempts newest first: history=%+v error=%v", history, err)
	}
	otherProject := expected
	otherProject.ID, otherProject.Project = uuid.New(), "another-project"
	otherProject.ContainerID, otherProject.Name = "other-docker-id", "other-qwen-name"
	if _, err := database.CreateDeployment(ctx, otherProject); err != nil {
		t.Fatalf("same model must be allowed in another project: %v", err)
	}
	for _, test := range []struct {
		name           string
		modelID        uuid.UUID
		deploymentName string
		constraint     string
	}{
		{"duplicate model", expected.ModelID, "another-deployment-name", "deployments_project_model_id_key"},
		{"duplicate deployment name", uuid.New(), expected.DeploymentName, "deployments_project_deployment_name_key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			savepoint, err := tx.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer savepoint.Rollback(context.Background())
			duplicate := expected
			duplicate.ID = uuid.New()
			duplicate.ContainerID, duplicate.Name = "duplicate-docker-id", "another-random-name"
			duplicate.ModelID = test.modelID
			duplicate.DeploymentName = test.deploymentName
			_, err = repository.NewDeploymentRepository(savepoint).CreateDeployment(ctx, duplicate)
			if !errors.Is(err, domain.ErrDeploymentAlreadyExists) {
				t.Fatalf("expected a duplicate deployment error, got %v", err)
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.ConstraintName != test.constraint {
				t.Fatalf("expected uniqueness constraint %s, got %v", test.constraint, err)
			}
		})
	}
	if _, err := database.CreateDeployment(ctx, expected); err == nil {
		t.Fatal("expected a duplicate deployment error")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("expected PostgreSQL uniqueness error, got %v", err)
		}
	}
}

// Техника: классы эквивалентности результата БД и ошибки зависимости.
func TestDeploymentRepositoryUnit(t *testing.T) {
	for _, method := range []string{"check", "exists", "create", "get id", "get name", "list active", "list all", "list states", "get state", "update status"} {
		for _, scenario := range []string{"success", "missing", "database error"} {
			t.Run(method+"/"+scenario, func(t *testing.T) {
				d := testutil.DeploymentMother{}.Pending()
				id := pgtype.UUID{Bytes: d.ID, Valid: true}
				modelID := pgtype.UUID{Bytes: d.ModelID, Valid: true}
				stamp := pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true}
				values := []any{id, d.ContainerID, modelID, d.Project, d.Username, d.Name, d.DeploymentName, string(d.Status), []byte(`{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"org/model"}}`), stamp, stamp}
				stateValues := []any{id, d.ContainerID, string(d.Status), d.Project, d.DeploymentName, d.Name}
				failure := errors.New("database error")
				var rowErr error
				if scenario == "missing" {
					rowErr = pgx.ErrNoRows
				}
				if scenario == "database error" {
					rowErr = failure
				}
				calls := 0
				db := &testutil.MockDBTX{
					QueryRowFunc: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
						calls++
						v := values
						if strings.Contains(sql, "CheckConnection") {
							v = []any{int32(1)}
						} else if strings.Contains(sql, "ActiveDeploymentExists") {
							v = []any{true}
						} else if strings.Contains(sql, "GetDeploymentState") {
							v = stateValues
						}
						return testutil.Row{Values: v, Err: rowErr}
					},
					QueryFunc: func(_ context.Context, sql string, _ ...interface{}) (pgx.Rows, error) {
						calls++
						if scenario == "database error" {
							return nil, failure
						}
						v := values
						if strings.Contains(sql, "ListDeploymentStates") {
							v = stateValues
						}
						rows := &testutil.Rows{}
						if scenario == "success" {
							rows.Data = [][]any{v}
						}
						return rows, nil
					},
					ExecFunc: func(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
						calls++
						if scenario == "database error" {
							return pgconn.CommandTag{}, failure
						}
						if scenario == "missing" {
							return pgconn.NewCommandTag("UPDATE 0"), nil
						}
						return pgconn.NewCommandTag("UPDATE 1"), nil
					},
				}
				repo := repository.NewDeploymentRepository(db)
				ctx := context.Background()
				var err error
				var got domain.Deployment
				switch method {
				case "check":
					err = repo.Check(ctx)
				case "exists":
					var exists bool
					exists, err = repo.ActiveDeploymentExists(ctx, d.Project, d.ModelID)
					if scenario == "success" && !exists {
						t.Fatal("expected active deployment")
					}
				case "create":
					got, err = repo.CreateDeployment(ctx, d)
				case "get id":
					got, err = repo.GetDeploymentByID(ctx, d.ID)
				case "get name":
					got, err = repo.GetDeploymentByName(ctx, d.Name)
				case "list active":
					var rows []domain.Deployment
					rows, err = repo.ListDeployments(ctx, 0, 10)
					if err == nil && (len(rows) == 1) != (scenario == "success") {
						t.Fatalf("rows=%+v", rows)
					}
				case "list all":
					var rows []domain.Deployment
					rows, err = repo.ListAllDeployments(ctx, 0, 10)
					if err == nil && (len(rows) == 1) != (scenario == "success") {
						t.Fatalf("rows=%+v", rows)
					}
				case "list states":
					var rows []domain.DeploymentState
					rows, err = repo.ListDeploymentStates(ctx)
					if err == nil && (len(rows) == 1) != (scenario == "success") {
						t.Fatalf("states=%+v", rows)
					}
				case "get state":
					_, err = repo.GetDeploymentState(ctx, d.ContainerID)
				case "update status":
					var updated bool
					updated, err = repo.UpdateDeploymentStatus(ctx, domain.DeploymentState{ID: d.ID, ContainerID: d.ContainerID, Status: domain.StatusPending}, domain.StatusStarted)
					if err == nil && updated != (scenario == "success") {
						t.Fatalf("updated=%v", updated)
					}
				}
				if calls != 1 {
					t.Fatalf("calls=%d", calls)
				}
				if scenario == "database error" && !errors.Is(err, failure) {
					t.Fatalf("err=%v", err)
				}
				if scenario == "success" && (err != nil || ((method == "create" || method == "get id" || method == "get name") && got.ID != d.ID)) {
					t.Fatalf("got=%+v err=%v", got, err)
				}
				if scenario == "missing" {
					switch method {
					case "get id", "get name", "get state":
						if !errors.Is(err, domain.ErrDeploymentNotFound) {
							t.Fatalf("err=%v", err)
						}
					case "check", "exists", "create":
						if !errors.Is(err, pgx.ErrNoRows) {
							t.Fatalf("err=%v", err)
						}
					default:
						if err != nil {
							t.Fatal(err)
						}
					}
				}
			})
		}
	}
}

// Техника: класс конфликта активного запуска.
func TestDeploymentCreateDuplicate(t *testing.T) {
	db := &testutil.MockDBTX{QueryRowFunc: func(context.Context, string, ...interface{}) pgx.Row {
		return testutil.Row{Err: &pgconn.PgError{Code: "23505", ConstraintName: "deployments_project_model_id_key"}}
	}}
	_, err := repository.NewDeploymentRepository(db).CreateDeployment(context.Background(), testutil.DeploymentMother{}.Pending())
	if !errors.Is(err, domain.ErrDeploymentAlreadyExists) {
		t.Fatalf("err=%v", err)
	}
}

// Техника: классы эквивалентности ошибок при чтении набора строк.
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
			db := &testutil.MockDBTX{QueryFunc: func(context.Context, string, ...interface{}) (pgx.Rows, error) { return rows, nil }}
			_, err := repository.NewDeploymentRepository(db).ListAllDeployments(context.Background(), 0, 10)
			if !errors.Is(err, failure) || !rows.Closed {
				t.Fatalf("err=%v closed=%v", err, rows.Closed)
			}
		})
	}
}
