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
	"go.uber.org/mock/gomock"
)

// AAA: Docker calls use GoMock and deployment persistence uses real PostgreSQL.
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
		const image = "llama:server"
		command := deploymentCommand()
		containerID := uuid.NewString()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		runtime := NewMockRuntimeClient(ctrl)
		gomock.InOrder(
			runtime.EXPECT().CreateContainer(gomock.Any(), image, gomock.Any(), command.Spec).Return(containerID, nil),
			runtime.EXPECT().StartContainer(gomock.Any(), containerID).Return(nil),
		)

		svc := service.NewDeploymentService(repository.NewDeploymentRepository(tx), config.ServiceConfig{InferenceServerImage: image}, runtime)
		created, err := svc.StartDeployment(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := svc.GetDeploymentByID(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if saved.ID == uuid.Nil || saved.ID != created.ID || saved.ModelID != command.ModelID || saved.Project != command.Project || saved.Status != domain.StatusPending || saved.ContainerID != containerID {
			t.Fatalf("incorrect persisted deployment: %+v", saved)
		}
	})
}
