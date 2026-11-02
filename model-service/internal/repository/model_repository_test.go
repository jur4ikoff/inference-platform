package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/repository"
	"github.com/jur4ikoff/model-service/internal/service"
)

func TestModelsAreIdentifiedByIDWithinProject(t *testing.T) {
	databaseURL := os.Getenv("MODEL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MODEL_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	config.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// The temporary table shadows the application's models table on this connection.
	// Closing the pool removes it without changing application data.
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE models (
		id UUID PRIMARY KEY,
		created_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		user_id UUID NOT NULL,
		project TEXT NOT NULL,
		spec JSONB NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../../infra/postgres/migrations/000006_unique_model_name_per_project.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, strings.ReplaceAll(string(migration), "-- ", "")); err != nil {
		t.Fatal(err)
	}
	runtime := &runtimeSpy{}
	models := service.NewModelService(repository.NewModelRepository(pool), runtime)
	projectA, projectB, missingProject := "demo-project", "another-project", "missing-project"
	create := func(project, name string) domain.Model {
		t.Helper()
		model, err := models.Create(ctx, service.CreateModelInput{
			Name: name, Project: project, UserID: uuid.NewString(), Spec: json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		return model
	}
	a, b := create(projectA, "demo"), create(projectB, "demo")
	create(projectA, "other")

	for _, expected := range []domain.Model{a, b} {
		actual, err := models.Get(ctx, " "+expected.Project+" ", " "+expected.ID+" ")
		if err != nil || actual.ID != expected.ID {
			t.Fatalf("get project %s: model=%+v error=%v", expected.Project, actual, err)
		}
	}
	if _, err := models.Create(ctx, service.CreateModelInput{
		Name: "demo", Project: projectA, UserID: uuid.NewString(), Spec: json.RawMessage(`{}`),
	}); !errors.Is(err, domain.ErrModelAlreadyExists) {
		t.Fatalf("duplicate name in project: %v", err)
	}

	name, description := "renamed", "updated"
	updated, err := models.Update(ctx, projectA, a.ID, service.UpdateModelInput{Name: &name, Description: &description})
	if err != nil || updated.ID != a.ID || updated.Name != name || updated.Description != description {
		t.Fatalf("rename: model=%+v error=%v", updated, err)
	}
	byID, err := models.Get(ctx, projectA, a.ID)
	if err != nil || byID.ID != a.ID || byID.Name != name {
		t.Fatalf("ID should resolve after rename: model=%+v error=%v", byID, err)
	}
	if _, err := models.Get(ctx, projectB, a.ID); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("ID should not resolve in another project: %v", err)
	}
	conflict := "other"
	if _, err := models.Update(ctx, projectA, a.ID, service.UpdateModelInput{Name: &conflict}); !errors.Is(err, domain.ErrModelAlreadyExists) {
		t.Fatalf("rename conflict: %v", err)
	}
	if _, err := models.Start(ctx, projectA, a.ID); err != nil || runtime.model.ID != a.ID {
		t.Fatalf("start selected wrong model: %+v error=%v", runtime.model, err)
	}

	if _, err := models.Get(ctx, missingProject, a.ID); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("get in wrong project: %v", err)
	}
	if _, err := models.Update(ctx, missingProject, a.ID, service.UpdateModelInput{Description: &description}); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("update in wrong project: %v", err)
	}
	if err := models.Delete(ctx, missingProject, a.ID); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("delete in wrong project: %v", err)
	}
	runtime.model = domain.Model{}
	if _, err := models.Start(ctx, missingProject, a.ID); !errors.Is(err, domain.ErrModelNotFound) || runtime.model.ID != "" {
		t.Fatalf("start in wrong project reached runtime: %+v error=%v", runtime.model, err)
	}
	if err := models.Delete(ctx, projectA, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := models.Get(ctx, projectA, a.ID); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("deleted model still resolves: %v", err)
	}
	remaining, err := models.Get(ctx, projectB, b.ID)
	if err != nil || remaining.ID != b.ID || remaining.Name != "demo" || remaining.Description != "" {
		t.Fatalf("another project was modified: %+v error=%v", remaining, err)
	}

	for _, selector := range [][2]string{{"", a.ID}, {projectA, " "}, {projectA, "demo"}} {
		if _, err := models.Get(ctx, selector[0], selector[1]); !errors.Is(err, service.ErrInvalidInput) {
			t.Errorf("get invalid selector: %v", err)
		}
		if _, err := models.Update(ctx, selector[0], selector[1], service.UpdateModelInput{Description: &description}); !errors.Is(err, service.ErrInvalidInput) {
			t.Errorf("update invalid selector: %v", err)
		}
		if err := models.Delete(ctx, selector[0], selector[1]); !errors.Is(err, service.ErrInvalidInput) {
			t.Errorf("delete invalid selector: %v", err)
		}
		if _, err := models.Start(ctx, selector[0], selector[1]); !errors.Is(err, service.ErrInvalidInput) {
			t.Errorf("start invalid selector: %v", err)
		}
	}
}

type runtimeSpy struct {
	model domain.Model
}

func (r *runtimeSpy) Start(_ context.Context, model domain.Model) error {
	r.model = model
	return nil
}
