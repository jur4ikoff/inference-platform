package service_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/repository"
	"github.com/jur4ikoff/inference-service/internal/service"
)

type lab2Runtime struct{ started string }

func (*lab2Runtime) EnsureImage(context.Context, string) error { return nil }
func (*lab2Runtime) CreateContainer(context.Context, string, string, domain.DeploymentSpec) (string, error) {
	return "lab2-" + uuid.NewString(), nil
}
func (r *lab2Runtime) StartContainer(_ context.Context, id string) error { r.started = id; return nil }
func (*lab2Runtime) RemoveContainer(context.Context, string) error       { return nil }
func (*lab2Runtime) StopContainer(context.Context, string) error         { return nil }

// AAA: only Docker is fake; deployment persistence uses real PostgreSQL.
func TestServiceIntegration(t *testing.T) {
	dsn := os.Getenv("INFERENCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("INFERENCE_TEST_DATABASE_URL is required for integration")
	}
	allure.Wrap(t, func(t *allure.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelCleanup()
			if err := tx.Rollback(cleanupCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Error(err)
			}
		}()
		runtime := &lab2Runtime{}
		svc := service.NewDeploymentService(repository.NewDeploymentRepository(tx), config.ServiceConfig{InferenceServerImage: "lab2-fixture"}, runtime)
		command := service.StartDeploymentCommand{Project: "lab2-integration", Username: "labadmin", ModelID: uuid.New(), DeploymentName: "lab2-model", Spec: domain.DeploymentSpec{ProviderType: domain.ModelProviderTypeHuggingFace, ProviderHuggingFace: &domain.HuggingFaceProvider{Repo: "lab2/fixture"}}}
		created, err := svc.StartDeployment(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := svc.GetDeploymentByID(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if saved.ID == uuid.Nil || saved.ID != created.ID || saved.ModelID != command.ModelID || saved.Project != command.Project || saved.Status != domain.StatusPending || saved.ContainerID != runtime.started {
			t.Fatalf("incorrect persisted deployment: %+v", saved)
		}
	})
}
