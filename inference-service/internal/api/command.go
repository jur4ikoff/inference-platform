package api

import (
	"strings"

	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/service"
	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func startDeploymentCommand(request *inferencev1.StartDeploymentRequest) (service.StartDeploymentCommand, error) {
	if err := validateSelector(request.GetProject(), request.GetModelId()); err != nil {
		return service.StartDeploymentCommand{}, err
	}
	modelID, err := uuid.Parse(request.GetModelId())
	if err != nil || modelID == uuid.Nil {
		return service.StartDeploymentCommand{}, status.Error(codes.InvalidArgument, "model_id must be a non-zero UUID")
	}
	username := strings.TrimSpace(request.GetUsername())
	if username == "" {
		return service.StartDeploymentCommand{}, status.Error(codes.InvalidArgument, "username is required")
	}
	deploymentName := strings.TrimSpace(request.GetDeploymentName())
	if deploymentName == "" {
		return service.StartDeploymentCommand{}, status.Error(codes.InvalidArgument, "deployment_name is required")
	}
	if !domain.ValidDNSLabel(strings.TrimSpace(request.GetProject())) {
		return service.StartDeploymentCommand{}, status.Error(codes.InvalidArgument, "project must contain 1-63 lowercase letters, digits or hyphens and must start and end with a letter or digit")
	}
	if !domain.ValidDNSLabel(deploymentName) {
		return service.StartDeploymentCommand{}, status.Error(codes.InvalidArgument, "deployment_name must contain 1-63 lowercase letters, digits or hyphens and must start and end with a letter or digit")
	}
	spec := request.GetSpec()
	if spec == nil {
		return service.StartDeploymentCommand{}, status.Error(codes.InvalidArgument, "spec is required")
	}
	if domain.ModelProviderType(spec.GetProviderType()) != domain.ModelProviderTypeHuggingFace {
		return service.StartDeploymentCommand{}, status.Error(codes.InvalidArgument, "unsupported spec.provider_type")
	}
	repo := strings.TrimSpace(spec.GetProviderHuggingFace().GetRepo())
	if repo == "" {
		return service.StartDeploymentCommand{}, status.Error(codes.InvalidArgument, "spec.provider_hugging_face.repo is required")
	}
	return service.StartDeploymentCommand{
		Project:        strings.TrimSpace(request.GetProject()),
		Username:       username,
		ModelID:        modelID,
		DeploymentName: deploymentName,
		Spec: domain.DeploymentSpec{
			ProviderType: domain.ModelProviderTypeHuggingFace,
			ProviderHuggingFace: &domain.HuggingFaceProvider{
				Repo: repo,
			},
		},
	}, nil
}
