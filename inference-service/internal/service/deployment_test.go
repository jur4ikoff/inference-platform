package service_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/service"
)

func deploymentCommand() service.StartDeploymentCommand {
	return service.StartDeploymentCommand{
		Project: "project-1", UserID: uuid.New(), ModelID: uuid.New(), DeploymentName: "qwen-small",
		Spec: domain.DeploymentSpec{
			ProviderType: domain.ModelProviderTypeHuggingFace,
			ProviderHuggingFace: &domain.HuggingFaceProvider{
				Repo: "Qwen/model",
			},
		},
	}
}

func TestStartDeploymentSavesCreatedDeployment(t *testing.T) {
	command := deploymentCommand()
	runtime := &deploymentRuntime{}
	repo := &deploymentRepository{}
	runtime.beforeCreate = func() {
		if !repo.checked || repo.checkedProject != command.Project || repo.checkedModelID != command.ModelID {
			t.Fatal("deployment must be checked by project and model ID before calling Docker")
		}
	}
	runtime.beforeStart = func() {
		if repo.calls != 1 || repo.saved.ContainerID != "docker-id" {
			t.Fatal("deployment must be saved before starting its Docker container")
		}
	}
	repo.create = func(_ context.Context, deployment domain.Deployment) (domain.Deployment, error) {
		if !runtime.created {
			t.Fatal("database was written before the Docker container was created")
		}
		return deployment, nil
	}
	svc := service.NewDeploymentService(repo, config.ServiceConfig{InferenceServerImage: "llama:server"}, runtime)
	deployment, err := svc.StartDeployment(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.ID == uuid.Nil || deployment.ContainerID != "docker-id" || deployment.ModelID != command.ModelID || deployment.Project != command.Project || deployment.Username != command.UserID.String() || deployment.Status != domain.StatusPending {
		t.Fatalf("incorrect deployment: %+v", deployment)
	}
	if !reflect.DeepEqual(deployment.Spec, command.Spec) || !reflect.DeepEqual(repo.saved, deployment) || repo.calls != 1 {
		t.Fatalf("saved deployment differs from the returned result: %+v", repo.saved)
	}
	if deployment.Name != runtime.name || !strings.HasPrefix(deployment.Name, command.DeploymentName+"-") || runtime.image != "llama:server" {
		t.Fatal("Docker and database have different deployment details")
	}
	if runtime.removedID != "" {
		t.Fatal("a successfully saved container was removed")
	}
	if runtime.startedID != deployment.ContainerID {
		t.Fatal("the saved deployment container was not started")
	}
}

func TestStartDeploymentRejectsExistingDeploymentBeforeDocker(t *testing.T) {
	runtime := &deploymentRuntime{}
	repo := &deploymentRepository{exists: true}
	svc := service.NewDeploymentService(repo, config.ServiceConfig{}, runtime)
	if _, err := svc.StartDeployment(context.Background(), deploymentCommand()); !errors.Is(err, domain.ErrDeploymentAlreadyExists) {
		t.Fatalf("expected an existing deployment error, got %v", err)
	}
	if runtime.created || runtime.removedID != "" || repo.calls != 0 {
		t.Fatal("an existing deployment must not create, save, or remove a container")
	}
}

func TestStartDeploymentStopsWhenDeploymentCheckFails(t *testing.T) {
	for _, failure := range []error{errors.New("database unavailable"), context.Canceled} {
		repo := &deploymentRepository{existsErr: failure}
		runtime := &deploymentRuntime{}
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, runtime)
		if _, err := svc.StartDeployment(context.Background(), deploymentCommand()); !errors.Is(err, failure) {
			t.Fatalf("expected existence check failure, got %v", err)
		}
		if runtime.created || repo.calls != 0 {
			t.Fatal("failed existence check must stop creation")
		}
	}
}

func TestStartDeploymentRemovesNewContainerAfterConcurrentDuplicate(t *testing.T) {
	repo := &deploymentRepository{create: func(context.Context, domain.Deployment) (domain.Deployment, error) {
		return domain.Deployment{}, domain.ErrDeploymentAlreadyExists
	}}
	runtime := &deploymentRuntime{}
	svc := service.NewDeploymentService(repo, config.ServiceConfig{}, runtime)
	if _, err := svc.StartDeployment(context.Background(), deploymentCommand()); !errors.Is(err, domain.ErrDeploymentAlreadyExists) {
		t.Fatalf("expected a concurrent duplicate error, got %v", err)
	}
	if !repo.checked || runtime.removedID != "docker-id" {
		t.Fatal("a duplicate inserted after the check must remove the newly created container")
	}
}

func TestStartDeploymentReturnsDeploymentFromRepository(t *testing.T) {
	var expected domain.Deployment
	repo := &deploymentRepository{create: func(_ context.Context, deployment domain.Deployment) (domain.Deployment, error) {
		deployment.Username = "database-username"
		expected = deployment
		return deployment, nil
	}}
	svc := service.NewDeploymentService(repo, config.ServiceConfig{}, &deploymentRuntime{})
	actual, err := svc.StartDeployment(context.Background(), deploymentCommand())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) || reflect.DeepEqual(actual, repo.saved) {
		t.Fatalf("service must return the repository result, got %+v, want %+v", actual, expected)
	}
}

func TestStartDeploymentDoesNotSaveFailedContainer(t *testing.T) {
	failure := errors.New("Docker unavailable")
	runtime := &deploymentRuntime{createErr: failure}
	repo := &deploymentRepository{}
	svc := service.NewDeploymentService(repo, config.ServiceConfig{}, runtime)
	if _, err := svc.StartDeployment(context.Background(), deploymentCommand()); !errors.Is(err, failure) {
		t.Fatalf("expected Docker failure, got %v", err)
	}
	if repo.calls != 0 || runtime.removedID != "" {
		t.Fatal("failed container creation reached persistence or cleanup")
	}
}

func TestStartDeploymentRemovesContainerWhenPersistenceFails(t *testing.T) {
	failure := errors.New("database write failed")
	for _, cleanupFailure := range []error{nil, errdefs.ErrNotFound, errors.New("Docker remove failed")} {
		t.Run("cleanup", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runtime := &deploymentRuntime{removeErr: cleanupFailure}
			repo := &deploymentRepository{create: func(context.Context, domain.Deployment) (domain.Deployment, error) {
				cancel()
				return domain.Deployment{}, failure
			}}
			svc := service.NewDeploymentService(repo, config.ServiceConfig{}, runtime)
			deployment, err := svc.StartDeployment(ctx, deploymentCommand())
			if !errors.Is(err, failure) || !reflect.DeepEqual(deployment, domain.Deployment{}) {
				t.Fatalf("expected persistence failure and an empty result, got %+v, %v", deployment, err)
			}
			if runtime.removedID != "docker-id" || runtime.cleanupCanceled || !runtime.cleanupHasDeadline {
				t.Fatal("cleanup must remove the new container with a separate, bounded context")
			}
			if cleanupFailure != nil && !errdefs.IsNotFound(cleanupFailure) && !errors.Is(err, cleanupFailure) {
				t.Fatalf("cleanup error was lost: %v", err)
			}
		})
	}
}

func TestStartDeploymentRequiresDatabaseBeforeCreatingContainer(t *testing.T) {
	runtime := &deploymentRuntime{}
	svc := service.NewDeploymentService(nil, config.ServiceConfig{}, runtime)
	if _, err := svc.StartDeployment(context.Background(), deploymentCommand()); !errors.Is(err, domain.ErrDatabaseUnavailable) {
		t.Fatalf("expected unavailable database, got %v", err)
	}
	if runtime.created {
		t.Fatal("container was created without a database repository")
	}
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
