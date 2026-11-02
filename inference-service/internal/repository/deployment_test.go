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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/repository"
	"github.com/jur4ikoff/utils/postgres"
)

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
	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "infra", "postgres", "migrations", "000011_create_deployments_table.sql"))
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Replace(string(migration), "CREATE TABLE deployments", "CREATE TEMP TABLE deployments", 1)
	if _, err := tx.Exec(ctx, schema); err != nil {
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
	if !reflect.DeepEqual(saved, expected) {
		t.Fatalf("returned deployment differs: got %+v, want %+v", saved, expected)
	}
	var actual domain.Deployment
	var spec []byte
	var specType, providerType, repo string
	if err := tx.QueryRow(ctx, `
		SELECT id, container_id, model_id, project, username, name, deployment_name, status, spec,
		       pg_typeof(spec)::text, spec->>'provider_type', spec->'provider_hugging_face'->>'repo'
		FROM deployments WHERE id = $1`, expected.ID).Scan(
		&actual.ID, &actual.ContainerID, &actual.ModelID, &actual.Project, &actual.Username,
		&actual.Name, &actual.DeploymentName, &actual.Status, &spec, &specType, &providerType, &repo,
	); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(spec, &actual.Spec); err != nil {
		t.Fatal(err)
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
	if history, err := database.ListAllDeployments(ctx, 0, 10); err != nil || len(history) != 2 {
		t.Fatalf("restarting must preserve the stopped deployment: history=%+v error=%v", history, err)
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
