package service_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/service"
	"github.com/jur4ikoff/inference-service/internal/testutil"

	"go.uber.org/mock/gomock"
)

func deploymentCommand() service.StartDeploymentCommand {
	return service.StartDeploymentCommand{
		Project: "project-1", Username: "alice", ModelID: uuid.New(), DeploymentName: "qwen-small",
		Spec: domain.DeploymentSpec{
			ProviderType: domain.ModelProviderTypeHuggingFace,
			ProviderHuggingFace: &domain.HuggingFaceProvider{
				Repo: "Qwen/model",
			},
		},
	}
}

// Техника: разбиение на классы эквивалентности; случай успешного создания.
func TestStartDeploymentSavesCreatedDeployment(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		command := deploymentCommand()
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		runtime := NewMockRuntimeClient(ctrl)
		var containerName string
		var saved domain.Deployment
		gomock.InOrder(
			repo.EXPECT().ActiveDeploymentExists(gomock.Any(), command.Project, command.ModelID).Return(false, nil),
			runtime.EXPECT().CreateContainer(gomock.Any(), "llama:server", gomock.Any(), command.Spec).
				DoAndReturn(func(_ context.Context, _ string, name string, _ domain.DeploymentSpec) (string, error) {
					containerName = name
					return "docker-id", nil
				}),
			repo.EXPECT().CreateDeployment(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, deployment domain.Deployment) (domain.Deployment, error) {
					saved = deployment
					return deployment, nil
				}),
			runtime.EXPECT().StartContainer(gomock.Any(), "docker-id").Return(nil),
		)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{InferenceServerImage: "llama:server"}, runtime)
		deployment, err := svc.StartDeployment(context.Background(), command)
		if err != nil {
			allureCtx.Fatal(err)
		}
		if deployment.ID == uuid.Nil || deployment.ContainerID != "docker-id" || deployment.ModelID != command.ModelID || deployment.Project != command.Project || deployment.Username != command.Username || deployment.Status != domain.StatusPending {
			allureCtx.Fatalf("incorrect deployment: %+v", deployment)
		}
		if !reflect.DeepEqual(deployment.Spec, command.Spec) || !reflect.DeepEqual(saved, deployment) {
			allureCtx.Fatalf("saved deployment differs from the returned result: %+v", saved)
		}
		if deployment.Name != containerName || !strings.HasPrefix(deployment.Name, command.DeploymentName+"-") {
			allureCtx.Fatal("Docker and database have different deployment details")
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай существующего активного запуска.
func TestStartDeploymentRejectsExistingDeploymentBeforeDocker(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		runtime := &deploymentRuntime{}
		repo := &deploymentRepository{exists: true}
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, runtime)
		if _, err := svc.StartDeployment(context.Background(), deploymentCommand()); !errors.Is(err, domain.ErrDeploymentAlreadyExists) {
			allureCtx.Fatalf("expected an existing deployment error, got %v", err)
		}
		if runtime.created || runtime.removedID != "" || repo.calls != 0 {
			allureCtx.Fatal("an existing deployment must not create, save, or remove a container")
		}
	})
}

// Техника: разбиение на классы эквивалентности ошибок проверки БД.
func TestStartDeploymentStopsWhenDeploymentCheckFails(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		for _, tc := range []struct {
			name    string
			failure error
		}{
			{"database unavailable", errors.New("database unavailable")},
			{"context canceled", context.Canceled},
		} {
			t.Run(tc.name, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					repo := &deploymentRepository{existsErr: tc.failure}
					runtime := &deploymentRuntime{}
					svc := service.NewDeploymentService(repo, config.ServiceConfig{}, runtime)
					if _, err := svc.StartDeployment(t.Context(), deploymentCommand()); !errors.Is(err, tc.failure) {
						allureCtx.Fatalf("expected existence check failure, got %v", err)
					}
					if runtime.created || repo.calls != 0 {
						allureCtx.Fatal("failed existence check must stop creation")
					}
				})
			})
		}
	})
}

// Техника: тестирование переходов состояний при конкурентном конфликте.
func TestStartDeploymentRemovesNewContainerAfterConcurrentDuplicate(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		repo := &deploymentRepository{create: func(context.Context, domain.Deployment) (domain.Deployment, error) {
			return domain.Deployment{}, domain.ErrDeploymentAlreadyExists
		}}
		runtime := &deploymentRuntime{}
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, runtime)
		if _, err := svc.StartDeployment(context.Background(), deploymentCommand()); !errors.Is(err, domain.ErrDeploymentAlreadyExists) {
			allureCtx.Fatalf("expected a concurrent duplicate error, got %v", err)
		}
		if !repo.checked || runtime.removedID != "docker-id" {
			allureCtx.Fatal("a duplicate inserted after the check must remove the newly created container")
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай успешно сохранённой записи.
func TestStartDeploymentReturnsDeploymentFromRepository(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		var expected domain.Deployment
		repo := &deploymentRepository{create: func(_ context.Context, deployment domain.Deployment) (domain.Deployment, error) {
			deployment.Username = "database-username"
			expected = deployment
			return deployment, nil
		}}
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, &deploymentRuntime{})
		actual, err := svc.StartDeployment(context.Background(), deploymentCommand())
		if err != nil {
			allureCtx.Fatal(err)
		}
		if !reflect.DeepEqual(actual, expected) || reflect.DeepEqual(actual, repo.saved) {
			allureCtx.Fatalf("service must return the repository result, got %+v, want %+v", actual, expected)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки создания контейнера.
func TestStartDeploymentDoesNotSaveFailedContainer(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("Docker unavailable")
		runtime := &deploymentRuntime{createErr: failure}
		repo := &deploymentRepository{}
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, runtime)
		if _, err := svc.StartDeployment(context.Background(), deploymentCommand()); !errors.Is(err, failure) {
			allureCtx.Fatalf("expected Docker failure, got %v", err)
		}
		if repo.calls != 0 || runtime.removedID != "" {
			allureCtx.Fatal("failed container creation reached persistence or cleanup")
		}
	})
}

// Техника: разбиение на классы эквивалентности ошибок сохранения и очистки.
func TestStartDeploymentRemovesContainerWhenPersistenceFails(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("database write failed")
		for _, tc := range []struct {
			name           string
			cleanupFailure error
		}{
			{"cleanup succeeds", nil},
			{"container already absent", errdefs.ErrNotFound},
			{"cleanup fails", errors.New("Docker remove failed")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					runtime := &deploymentRuntime{removeErr: tc.cleanupFailure}
					repo := &deploymentRepository{create: func(context.Context, domain.Deployment) (domain.Deployment, error) {
						cancel()
						return domain.Deployment{}, failure
					}}
					svc := service.NewDeploymentService(repo, config.ServiceConfig{}, runtime)
					deployment, err := svc.StartDeployment(ctx, deploymentCommand())
					if !errors.Is(err, failure) || !reflect.DeepEqual(deployment, domain.Deployment{}) {
						allureCtx.Fatalf("expected persistence failure and an empty result, got %+v, %v", deployment, err)
					}
					if runtime.removedID != "docker-id" || runtime.cleanupCanceled || !runtime.cleanupHasDeadline {
						allureCtx.Fatal("cleanup must remove the new container with a separate, bounded context")
					}
					if tc.cleanupFailure != nil && !errdefs.IsNotFound(tc.cleanupFailure) && !errors.Is(err, tc.cleanupFailure) {
						allureCtx.Fatalf("cleanup error was lost: %v", err)
					}
				})
			})
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай отсутствующей зависимости БД.
func TestStartDeploymentRequiresDatabaseBeforeCreatingContainer(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		runtime := &deploymentRuntime{}
		svc := service.NewDeploymentService(nil, config.ServiceConfig{}, runtime)
		if _, err := svc.StartDeployment(context.Background(), deploymentCommand()); !errors.Is(err, domain.ErrDatabaseUnavailable) {
			allureCtx.Fatalf("expected unavailable database, got %v", err)
		}
		if runtime.created {
			allureCtx.Fatal("container was created without a database repository")
		}
	})
}

type deploymentRepository struct {
	service.DeploymentRepository
	exists         bool
	existsErr      error
	checked        bool
	checkedProject string
	checkedModelID uuid.UUID
	saved          domain.Deployment
	calls          int
	create         func(context.Context, domain.Deployment) (domain.Deployment, error)
}

func (r *deploymentRepository) ActiveDeploymentExists(_ context.Context, project string, modelID uuid.UUID) (bool, error) {
	r.checked, r.checkedProject, r.checkedModelID = true, project, modelID
	return r.exists, r.existsErr
}

func (r *deploymentRepository) CreateDeployment(ctx context.Context, deployment domain.Deployment) (domain.Deployment, error) {
	r.saved = deployment
	r.calls++
	if r.create != nil {
		return r.create(ctx, deployment)
	}
	return deployment, nil
}

type deploymentRuntime struct {
	service.RuntimeClient
	created            bool
	beforeCreate       func()
	beforeStart        func()
	startedID          string
	image, name        string
	createErr          error
	removedID          string
	removeErr          error
	cleanupCanceled    bool
	cleanupHasDeadline bool
}

func (r *deploymentRuntime) CreateContainer(_ context.Context, image, name string, _ domain.DeploymentSpec) (string, error) {
	if r.beforeCreate != nil {
		r.beforeCreate()
	}
	r.created, r.image, r.name = true, image, name
	return "docker-id", r.createErr
}

func (r *deploymentRuntime) StartContainer(_ context.Context, id string) error {
	if r.beforeStart != nil {
		r.beforeStart()
	}
	r.startedID = id
	return nil
}

func (r *deploymentRuntime) RemoveContainer(ctx context.Context, id string) error {
	r.removedID = id
	r.cleanupCanceled = ctx.Err() != nil
	_, r.cleanupHasDeadline = ctx.Deadline()
	return r.removeErr
}

// Техника: разбиение на классы эквивалентности; случай успешного ответа runtime.
func TestDeploymentInitEnsuresImage(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		rt := NewMockRuntimeClient(ctrl)
		rt.EXPECT().EnsureImage(gomock.Any(), "llama:server").Return(nil)
		svc := service.NewDeploymentService(nil, config.ServiceConfig{InferenceServerImage: "llama:server"}, rt)
		if err := svc.Init(t.Context()); err != nil {
			allureCtx.Fatal(err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки runtime.
func TestDeploymentInitReportsRuntimeError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("runtime unavailable")
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		rt := NewMockRuntimeClient(ctrl)
		rt.EXPECT().EnsureImage(gomock.Any(), "llama:server").Return(failure)
		svc := service.NewDeploymentService(nil, config.ServiceConfig{InferenceServerImage: "llama:server"}, rt)
		if err := svc.Init(t.Context()); !errors.Is(err, failure) {
			allureCtx.Fatalf("err=%v want=%v", err, failure)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай найденного deployment.
func TestGetDeploymentByIDReturnsDeployment(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		d := testutil.DeploymentMother{}.Running()
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		repo.EXPECT().GetDeploymentByID(gomock.Any(), d.ID).Return(d, nil)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, nil)
		got, err := svc.GetDeploymentByID(t.Context(), d.ID)
		if err != nil || !reflect.DeepEqual(got, d) {
			allureCtx.Fatalf("got=%+v err=%v", got, err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай нулевого UUID.
func TestGetDeploymentByIDRejectsEmptyID(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, nil)
		_, err := svc.GetDeploymentByID(t.Context(), uuid.Nil)
		if !errors.Is(err, domain.ErrInvalidDeploymentID) {
			allureCtx.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай найденного deployment.
func TestGetDeploymentByNameReturnsDeployment(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		d := testutil.DeploymentMother{}.Running()
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		repo.EXPECT().GetDeploymentByName(gomock.Any(), d.Name).Return(d, nil)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, nil)
		got, err := svc.GetDeploymentByName(t.Context(), d.Name)
		if err != nil || !reflect.DeepEqual(got, d) {
			allureCtx.Fatalf("got=%+v err=%v", got, err)
		}
	})
}

// Техника: анализ граничных значений; пустого имени.
func TestGetDeploymentByNameRejectsEmptyName(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, nil)
		_, err := svc.GetDeploymentByName(t.Context(), " ")
		if !errors.Is(err, domain.ErrInvalidDeploymentName) {
			allureCtx.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай успешной остановки контейнера.
func TestStopDeploymentStopsContainer(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		d := testutil.DeploymentMother{}.Running()
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		rt := NewMockRuntimeClient(ctrl)
		repo.EXPECT().GetDeploymentByID(gomock.Any(), d.ID).Return(d, nil)
		rt.EXPECT().StopContainer(gomock.Any(), d.ContainerID).Return(nil)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, rt)
		if err := svc.StopDeployment(t.Context(), d.ID); err != nil {
			allureCtx.Fatal(err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки остановки контейнера.
func TestStopDeploymentReportsRuntimeError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		d := testutil.DeploymentMother{}.Running()
		failure := errors.New("runtime unavailable")
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		rt := NewMockRuntimeClient(ctrl)
		repo.EXPECT().GetDeploymentByID(gomock.Any(), d.ID).Return(d, nil)
		rt.EXPECT().StopContainer(gomock.Any(), d.ContainerID).Return(failure)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, rt)
		if err := svc.StopDeployment(t.Context(), d.ID); !errors.Is(err, failure) {
			allureCtx.Fatalf("err=%v want=%v", err, failure)
		}
	})
}
