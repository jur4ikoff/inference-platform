package service

import (
	"context"
	"errors"
	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/testutil"
	"testing"
	"time"
)

type deleteRepositoryStub struct {
	ModelRepository
	deleted bool
	getErr  error
}

func (r *deleteRepositoryStub) Get(_ context.Context, project, id string) (domain.Model, error) {
	return domain.Model{ID: id, Project: project}, r.getErr
}
func (r *deleteRepositoryStub) Delete(context.Context, string, string) error {
	r.deleted = true
	return nil
}

// Техника: переходы состояний deployment.
func TestDeleteRequiresAllDeploymentsStopped(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		const modelID = "e75957e1-6ac2-403c-8154-103d8d9c333d"
		for _, tc := range []struct {
			name                string
			history             []domain.Deployment
			runtimeErr, wantErr error
			deleted             bool
		}{
			{name: "no deployments", deleted: true},
			{name: "terminal deployments", history: []domain.Deployment{{Project: "ml", ModelID: modelID, Status: "stopped"}, {Project: "ml", ModelID: modelID, Status: "failed"}}, deleted: true},
			{name: "other model", history: []domain.Deployment{{Project: "ml", ModelID: "other", Status: "running"}}, deleted: true},
			{name: "other project", history: []domain.Deployment{{Project: "other", ModelID: modelID, Status: "running"}}, deleted: true},
			{name: "running", history: []domain.Deployment{{Project: "ml", ModelID: modelID, Status: "running"}}, wantErr: ErrActiveDeployments},
			{name: "stopping", history: []domain.Deployment{{Project: "ml", ModelID: modelID, Status: "stopping"}}, wantErr: ErrActiveDeployments},
			{name: "unknown", history: []domain.Deployment{{Project: "ml", ModelID: modelID, Status: "unknown"}}, wantErr: ErrActiveDeployments},
			{name: "runtime unavailable", runtimeErr: ErrRuntimeUnavailable, wantErr: ErrRuntimeUnavailable},
		} {
			t.Run(tc.name, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					repo := &deleteRepositoryStub{}
					svc := NewModelService(repo, historyRuntimeStub{history: tc.history, err: tc.runtimeErr})
					err := svc.Delete(context.Background(), "ml", modelID)
					if !errors.Is(err, tc.wantErr) || repo.deleted != tc.deleted {
						allureCtx.Fatalf("error=%v deleted=%v", err, repo.deleted)
					}
				})
			})
		}
	})
}

// Техника: классы эквивалентности входных данных.
func TestModelUseCases(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		for _, method := range []string{"create", "get", "update", "start", "create deployment", "get deployment", "stop deployment", "history", "active"} {
			for _, valid := range []bool{true, false} {
				t.Run(method+map[bool]string{true: "/valid", false: "/invalid"}[valid], func(t *testing.T) {
					allure.Wrap(t, func(allureCtx *allure.Context) {
						ctx := context.Background()
						model := testutil.ModelMother{}.HuggingFace()
						deployment := testutil.DeploymentMother{}.Running()
						calls := 0
						repo := &testutil.MockModelRepository{CreateFunc: func(_ context.Context, m domain.Model) (domain.Model, error) { calls++; return m, nil }, GetFunc: func(context.Context, string, string) (domain.Model, error) { calls++; return model, nil }, UpdateFunc: func(context.Context, string, string, *string, *string, *domain.ModelSpec, time.Time) (domain.Model, error) {
							calls++
							return model, nil
						}}
						runtime := &testutil.MockModelRuntime{StartFunc: func(context.Context, domain.Model) error { return nil }, CreateDeploymentFunc: func(context.Context, domain.Model) (domain.Deployment, error) { return deployment, nil }, GetDeploymentFunc: func(context.Context, domain.Model) (domain.Deployment, error) { return deployment, nil }, StopDeploymentFunc: func(context.Context, domain.Deployment) error { return nil }, GetDeploymentsHistoryFunc: func(context.Context, string) ([]domain.Deployment, error) {
							return []domain.Deployment{deployment}, nil
						}, GetActiveDeploymentsFunc: func(context.Context, string) ([]domain.Deployment, error) {
							return []domain.Deployment{deployment}, nil
						}}
						svc := NewModelService(repo, runtime)
						project := model.Project
						if !valid {
							project = ""
						}
						name := "renamed"
						var err error
						switch method {
						case "create":
							_, err = svc.Create(ctx, CreateModelInput{Name: model.Name, Username: model.Username, Project: project, Spec: model.Spec})
						case "get":
							_, err = svc.Get(ctx, project, model.ID)
						case "update":
							_, err = svc.Update(ctx, project, model.ID, UpdateModelInput{Name: &name})
						case "start":
							_, err = svc.Start(ctx, project, model.ID)
						case "create deployment":
							_, err = svc.CreateDeployment(ctx, project, model.ID)
						case "get deployment":
							_, err = svc.GetDeployment(ctx, project, model.ID)
						case "stop deployment":
							_, err = svc.StopDeployment(ctx, project, model.ID)
						case "history":
							_, err = svc.GetDeploymentsHistory(ctx, project)
						case "active":
							_, err = svc.GetActiveDeployments(ctx, project)
						}
						if valid && err != nil {
							allureCtx.Fatal(err)
						}
						if !valid && (!errors.Is(err, ErrInvalidInput) || calls != 0) {
							allureCtx.Fatalf("err=%v calls=%d", err, calls)
						}
					})
				})
			}
		}
	})
}

// Техника: переходы состояний deployment.
func TestStopAlreadyStopped(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		model := testutil.ModelMother{}.HuggingFace()
		deployment := testutil.DeploymentMother{}.Stopped()
		repo := &testutil.MockModelRepository{GetFunc: func(context.Context, string, string) (domain.Model, error) { return model, nil }}
		runtime := &testutil.MockModelRuntime{GetDeploymentFunc: func(context.Context, domain.Model) (domain.Deployment, error) { return deployment, nil }}
		accepted, err := NewModelService(repo, runtime).StopDeployment(context.Background(), model.Project, model.ID)
		if err != nil || accepted {
			allureCtx.Fatalf("accepted=%v err=%v", accepted, err)
		}
	})
}

// Техника: классы эквивалентности ошибок зависимостей.
func TestModelDependencyErrors(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("dependency unavailable")
		model := testutil.ModelMother{}.HuggingFace()
		deployment := testutil.DeploymentMother{}.Running()
		for _, method := range []string{"create", "get", "update", "delete", "start", "create deployment", "get deployment", "stop deployment", "history", "active"} {
			t.Run(method, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					repo := &testutil.MockModelRepository{CreateFunc: func(context.Context, domain.Model) (domain.Model, error) { return domain.Model{}, failure }, GetFunc: func(context.Context, string, string) (domain.Model, error) {
						if method == "get" || method == "delete" {
							return domain.Model{}, failure
						}
						return model, nil
					}, UpdateFunc: func(context.Context, string, string, *string, *string, *domain.ModelSpec, time.Time) (domain.Model, error) {
						return domain.Model{}, failure
					}}
					runtime := &testutil.MockModelRuntime{StartFunc: func(context.Context, domain.Model) error { return failure }, CreateDeploymentFunc: func(context.Context, domain.Model) (domain.Deployment, error) { return domain.Deployment{}, failure }, GetDeploymentFunc: func(context.Context, domain.Model) (domain.Deployment, error) {
						if method == "stop deployment" {
							return deployment, nil
						}
						return domain.Deployment{}, failure
					}, StopDeploymentFunc: func(context.Context, domain.Deployment) error { return failure }, GetDeploymentsHistoryFunc: func(context.Context, string) ([]domain.Deployment, error) { return nil, failure }, GetActiveDeploymentsFunc: func(context.Context, string) ([]domain.Deployment, error) { return nil, failure }}
					svc := NewModelService(repo, runtime)
					ctx := context.Background()
					name := "renamed"
					var err error
					switch method {
					case "create":
						_, err = svc.Create(ctx, CreateModelInput{Name: model.Name, Username: model.Username, Project: model.Project, Spec: model.Spec})
					case "get":
						_, err = svc.Get(ctx, model.Project, model.ID)
					case "update":
						_, err = svc.Update(ctx, model.Project, model.ID, UpdateModelInput{Name: &name})
					case "delete":
						err = svc.Delete(ctx, model.Project, model.ID)
					case "start":
						_, err = svc.Start(ctx, model.Project, model.ID)
					case "create deployment":
						_, err = svc.CreateDeployment(ctx, model.Project, model.ID)
					case "get deployment":
						_, err = svc.GetDeployment(ctx, model.Project, model.ID)
					case "stop deployment":
						_, err = svc.StopDeployment(ctx, model.Project, model.ID)
					case "history":
						_, err = svc.GetDeploymentsHistory(ctx, model.Project)
					case "active":
						_, err = svc.GetActiveDeployments(ctx, model.Project)
					}
					if !errors.Is(err, failure) {
						allureCtx.Fatalf("error=%v", err)
					}
				})
			})
		}
	})
}

type memoryModelRepository struct {
	ModelRepository
	models map[string]domain.Model
}

func (r *memoryModelRepository) Create(_ context.Context, m domain.Model) (domain.Model, error) {
	for _, old := range r.models {
		if old.Project == m.Project && old.Name == m.Name {
			return domain.Model{}, domain.ErrModelAlreadyExists
		}
	}
	r.models[m.ID] = m
	return m, nil
}

// Техника: классы эквивалентности уникальности имени; классический стиль с репозиторием в памяти.
func TestCreateModelWithInMemoryRepository(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		for _, duplicate := range []bool{false, true} {
			t.Run(map[bool]string{false: "new", true: "duplicate"}[duplicate], func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					fixture := testutil.ModelMother{}.HuggingFace()
					repo := &memoryModelRepository{models: map[string]domain.Model{}}
					if duplicate {
						repo.models[fixture.ID] = fixture
					}
					input := CreateModelInput{Name: fixture.Name, Username: fixture.Username, Project: fixture.Project, Spec: fixture.Spec}
					result, err := NewModelService(repo, nil).Create(context.Background(), input)
					if duplicate && !errors.Is(err, domain.ErrModelAlreadyExists) {
						allureCtx.Fatalf("error=%v", err)
					}
					if !duplicate && (err != nil || repo.models[result.ID].Name != fixture.Name) {
						allureCtx.Fatalf("result=%+v err=%v", result, err)
					}
				})
			})
		}
	})
}

var _ ModelRepository = (*testutil.MockModelRepository)(nil)
var _ ModelRuntime = (*testutil.MockModelRuntime)(nil)
