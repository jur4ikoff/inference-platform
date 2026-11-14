package service

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=deployment.go -destination=deployment_mock_test.go -package=service_test -mock_names=DeploymentRepository=MockDeploymentRepository,RuntimeClient=MockRuntimeClient

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/rs/zerolog/log"
)

type DeploymentRepository interface {
	ActiveDeploymentExists(context.Context, string, uuid.UUID) (bool, error)
	CreateDeployment(context.Context, domain.Deployment) (domain.Deployment, error)
	GetDeploymentByID(context.Context, uuid.UUID) (domain.Deployment, error)
	GetDeploymentByName(context.Context, string) (domain.Deployment, error)
	ListDeployments(context.Context, int, int) ([]domain.Deployment, error)
	ListAllDeployments(context.Context, int, int) ([]domain.Deployment, error)
	UpdateDeploymentStatus(context.Context, domain.DeploymentState, domain.Status) (bool, error)
}

type RuntimeClient interface {
	EnsureImage(context.Context, string) error
	CreateContainer(ctx context.Context, image, containerName string, spec domain.DeploymentSpec) (string, error)
	StartContainer(context.Context, string) error
	RemoveContainer(context.Context, string) error
	StopContainer(context.Context, string) error
}

type DeploymentService struct {
	repo    DeploymentRepository
	config  config.ServiceConfig
	runtime RuntimeClient
}

func NewDeploymentService(repo DeploymentRepository, cfg config.ServiceConfig, runtime RuntimeClient) *DeploymentService {
	return &DeploymentService{
		repo:    repo,
		config:  cfg,
		runtime: runtime,
	}
}

func (s *DeploymentService) Init(ctx context.Context) (resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Err(resultErr).Str("component", "DeploymentService").Msg("Init failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "DeploymentService").Msg("Init completed")
	}()
	if err := s.runtime.EnsureImage(ctx, s.config.InferenceServerImage); err != nil {
		return fmt.Errorf("ensure inference server image: %w", err)
	}
	return nil
}

type StartDeploymentCommand struct {
	Project        string
	Username       string
	ModelID        uuid.UUID
	DeploymentName string
	Spec           domain.DeploymentSpec
}

func (s *DeploymentService) StartDeployment(ctx context.Context, command StartDeploymentCommand) (_ domain.Deployment, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Err(resultErr).Str("component", "DeploymentService").Msg("StartDeployment failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "DeploymentService").Msg("StartDeployment completed")
	}()
	if s.repo == nil {
		return domain.Deployment{}, domain.ErrDatabaseUnavailable
	}
	exists, err := s.repo.ActiveDeploymentExists(ctx, command.Project, command.ModelID)
	if err != nil {
		return domain.Deployment{}, fmt.Errorf("check existing deployment: %w", err)
	}
	if exists {
		return domain.Deployment{}, domain.ErrDeploymentAlreadyExists
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return domain.Deployment{}, fmt.Errorf("generate deployment ID: %w", err)
	}
	containerName := generateContainerName(command.DeploymentName)

	containerID, err := s.runtime.CreateContainer(ctx, s.config.InferenceServerImage, containerName, command.Spec)
	if err != nil {
		return domain.Deployment{}, fmt.Errorf("failed to create model container: %w", err)
	}
	deployment := domain.Deployment{
		ID:             id,
		ContainerID:    containerID,
		ModelID:        command.ModelID,
		Project:        command.Project,
		Username:       command.Username,
		Name:           containerName,
		DeploymentName: command.DeploymentName,
		Status:         domain.StatusPending,
		Spec:           command.Spec,
	}
	savedDeployment, err := s.repo.CreateDeployment(ctx, deployment)
	if err != nil {
		persistErr := fmt.Errorf("save deployment: %w", err)
		cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancelCleanup()
		if cleanupErr := s.runtime.RemoveContainer(cleanupCtx, containerID); cleanupErr != nil && !errdefs.IsNotFound(cleanupErr) {
			return domain.Deployment{}, errors.Join(persistErr, fmt.Errorf("remove unsaved deployment container %s: %w", containerID, cleanupErr))
		}
		return domain.Deployment{}, persistErr
	}
	log.Ctx(ctx).Info().Str("deployment_id", savedDeployment.ID.String()).Str("container_id", savedDeployment.ContainerID).Msg("deployment created")
	if err := s.runtime.StartContainer(ctx, savedDeployment.ContainerID); err != nil {
		startErr := fmt.Errorf("start deployment container: %w", err)
		statusCtx, cancelStatus := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancelStatus()
		previous := domain.DeploymentState{
			ID: savedDeployment.ID, ContainerID: savedDeployment.ContainerID, Status: savedDeployment.Status,
			Project: savedDeployment.Project, DeploymentName: savedDeployment.DeploymentName, Name: savedDeployment.Name,
		}
		if _, statusErr := s.repo.UpdateDeploymentStatus(statusCtx, previous, domain.StatusFailed); statusErr != nil {
			return domain.Deployment{}, errors.Join(startErr, fmt.Errorf("mark deployment failed: %w", statusErr))
		}
		return domain.Deployment{}, startErr
	}

	return savedDeployment, nil
}

func (s *DeploymentService) GetDeploymentByID(ctx context.Context, id uuid.UUID) (_ domain.Deployment, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Err(resultErr).Str("component", "DeploymentService").Msg("GetDeploymentByID failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "DeploymentService").Msg("GetDeploymentByID completed")
	}()
	if id == uuid.Nil {
		return domain.Deployment{}, domain.ErrInvalidDeploymentID
	}
	if s.repo == nil {
		return domain.Deployment{}, domain.ErrDatabaseUnavailable
	}
	return s.repo.GetDeploymentByID(ctx, id)
}

func (s *DeploymentService) GetDeploymentByName(ctx context.Context, name string) (_ domain.Deployment, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Err(resultErr).Str("component", "DeploymentService").Msg("GetDeploymentByName failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "DeploymentService").Msg("GetDeploymentByName completed")
	}()
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Deployment{}, domain.ErrInvalidDeploymentName
	}
	if s.repo == nil {
		return domain.Deployment{}, domain.ErrDatabaseUnavailable
	}
	return s.repo.GetDeploymentByName(ctx, name)
}

func (s *DeploymentService) StopDeployment(ctx context.Context, id uuid.UUID) (resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Err(resultErr).Str("component", "DeploymentService").Msg("StopDeployment failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "DeploymentService").Msg("StopDeployment completed")
	}()
	deployment, err := s.GetDeploymentByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get deployment to stop: %w", err)
	}
	if err := s.runtime.StopContainer(ctx, deployment.ContainerID); err != nil && !errdefs.IsNotFound(err) && !errdefs.IsNotModified(err) {
		return fmt.Errorf("stop deployment container: %w", err)
	}
	log.Ctx(ctx).Info().Str("deployment_id", id.String()).Str("container_id", deployment.ContainerID).Msg("deployment stop completed")
	return nil
}

func generateContainerName(deploymentName string) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	var suffix [5]byte
	for i := range suffix {
		suffix[i] = letters[rand.IntN(len(letters))]
	}
	return deploymentName + "-" + string(suffix[:])
}
