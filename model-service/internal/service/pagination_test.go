package service

import (
	"context"
	"errors"
	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/testutil"
	"testing"
)

type listRepositoryStub struct {
	ModelRepository
	models []domain.Model
}

func (r listRepositoryStub) List(context.Context, string, int, int) ([]domain.Model, error) {
	return r.models, nil
}

type historyRuntimeStub struct {
	ModelRuntime
	history []domain.Deployment
	err     error
}

func (r historyRuntimeStub) GetDeploymentsHistory(context.Context, string) ([]domain.Deployment, error) {
	return r.history, r.err
}

// Техника: классы эквивалентности состояний последнего запуска.
func TestListUsesLatestDeploymentForEachModel(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		repo := listRepositoryStub{models: []domain.Model{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}}
		runtime := historyRuntimeStub{history: []domain.Deployment{{ModelID: "a", Status: "starting"}, {ModelID: "b", Status: "running"}, {ModelID: "a", Status: "failed"}, {ModelID: "c", Status: "stopping"}}}
		page, err := NewModelService(repo, runtime).List(context.Background(), ListModelsInput{Project: "demo"})
		if err != nil {
			allureCtx.Fatal(err)
		}
		for id, want := range map[string]string{"a": "starting", "b": "running", "c": "stopping", "d": "stopped"} {
			if page.Statuses[id] != want {
				allureCtx.Errorf("%s: got %s, want %s", id, page.Statuses[id], want)
			}
		}

	})
}

// Техника: класс ошибки зависимости.
func TestListPropagatesRuntimeFailure(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		repo := listRepositoryStub{models: []domain.Model{{ID: "a"}}}
		_, err := NewModelService(repo, historyRuntimeStub{err: ErrRuntimeUnavailable}).List(context.Background(), ListModelsInput{Project: "demo"})
		if !errors.Is(err, ErrRuntimeUnavailable) {
			allureCtx.Fatalf("got %v", err)
		}

	})
}

// Техника: классы эквивалентности состояний deployment.
func TestModelStatus(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for input, want := range map[string]string{"pending": "starting", "starting": "starting", "running": "running", "stopping": "stopping", "stopped": "stopped", "failed": "failed", "other": "unknown"} {
			if got := modelStatus(input); got != want {
				allureCtx.Errorf("%s: got %s, want %s", input, got, want)
			}
		}

	})
}

// Техника: граничные значения размера страницы.
func TestListModels(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for _, valid := range []bool{true, false} {
			t.Run(map[bool]string{true: "valid", false: "invalid"}[valid], func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {

					model := testutil.ModelMother{}.HuggingFace()
					repo := &testutil.MockModelRepository{ListFunc: func(context.Context, string, int, int) ([]domain.Model, error) {
						return []domain.Model{model, model}, nil
					}}
					runtime := &testutil.MockModelRuntime{GetDeploymentsHistoryFunc: func(context.Context, string) ([]domain.Deployment, error) { return nil, nil }}
					limit := 1
					if !valid {
						limit = 101
					}
					page, err := NewModelService(repo, runtime).List(context.Background(), ListModelsInput{Project: "demo", Limit: limit})
					if valid && (err != nil || len(page.Models) != 1 || page.NextPageToken == nil || *page.NextPageToken != 1) {
						allureCtx.Fatalf("page=%+v err=%v", page, err)
					}
					if !valid && !errors.Is(err, ErrInvalidInput) {
						allureCtx.Fatalf("err=%v", err)
					}

				})
			})
		}

	})
}

// Техника: классы эквивалентности ошибок репозитория.
func TestListModelsRepositoryError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		failure := errors.New("repository unavailable")
		repo := &testutil.MockModelRepository{ListFunc: func(context.Context, string, int, int) ([]domain.Model, error) { return nil, failure }}
		_, err := NewModelService(repo, nil).List(context.Background(), ListModelsInput{Project: "demo"})
		if !errors.Is(err, failure) {
			allureCtx.Fatalf("err=%v", err)
		}

	})
}
