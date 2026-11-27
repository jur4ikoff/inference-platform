package service_test

import (
	"context"
	"os"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/repository"
	"github.com/jur4ikoff/model-service/internal/service"
)

// AAA: model creation and retrieval cross the production service and repository.
func TestServiceIntegration(t *testing.T) {
	dsn := os.Getenv("MODEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_TEST_DATABASE_URL is required for integration")
	}
	allure.Wrap(t, func(t *allure.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		const modelRepo = "integration/fixture"
		svc := service.NewModelService(repository.NewModelRepository(pool), nil)
		created, err := svc.Create(ctx, service.CreateModelInput{Name: "integration-model", Username: "testadmin", Project: "integration-project", Spec: domain.ModelSpec{ProviderType: domain.ProviderHuggingFace, ProviderHuggingFace: &domain.HuggingFaceProvider{Repo: modelRepo}}})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelCleanup()
			if _, err := pool.Exec(cleanupCtx, "DELETE FROM models WHERE id=$1", created.ID); err != nil {
				t.Error(err)
			}
		}()
		saved, err := svc.Get(ctx, created.Project, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if saved.ID == "" || saved.ID != created.ID || saved.Project != created.Project || saved.Name != created.Name || saved.Spec.ProviderHuggingFace.Repo != modelRepo {
			t.Fatalf("incorrect saved model: %+v", saved)
		}
	})
}
