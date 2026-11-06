package api

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/domain"
	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func validStartDeploymentRequest() *inferencev1.StartDeploymentRequest {
	return &inferencev1.StartDeploymentRequest{
		Project:        "project-1",
		ModelId:        "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1",
		Username:       "alice",
		DeploymentName: "qwen-small",
		Spec: &inferencev1.DeploymentSpec{
			ProviderType: "HUGGING_FACE",
			ProviderHuggingFace: &inferencev1.HuggingFaceProvider{
				Repo: "Qwen/model",
			},
		},
	}
}

// Техника: класс корректной protobuf-команды.
func TestStartDeploymentCommandFromProtobuf(t *testing.T) {
	request := validStartDeploymentRequest()
	data, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var received inferencev1.StartDeploymentRequest
	if err := proto.Unmarshal(data, &received); err != nil {
		t.Fatal(err)
	}
	command, err := startDeploymentCommand(&received)
	if err != nil {
		t.Fatal(err)
	}
	if command.Project != request.Project || command.Username != request.Username || command.ModelID.String() != request.ModelId || command.DeploymentName != request.DeploymentName {
		t.Fatalf("request identifiers were not preserved: %+v", command)
	}
	if command.Spec.ProviderType != domain.ModelProviderTypeHuggingFace || command.Spec.ProviderHuggingFace == nil || command.Spec.ProviderHuggingFace.Repo != "Qwen/model" {
		t.Fatalf("provider specification was not decoded: %+v", command.Spec)
	}
}

// Техника: классы эквивалентности и граничные значения данных команды.
func TestStartDeploymentRejectsInvalidCommandBeforeService(t *testing.T) {
	validSpec := &inferencev1.DeploymentSpec{
		ProviderType:        "HUGGING_FACE",
		ProviderHuggingFace: &inferencev1.HuggingFaceProvider{Repo: "Qwen/model"},
	}
	const modelID = "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1"
	tests := []struct {
		name           string
		project        string
		modelID        string
		username       string
		deploymentName string
		spec           *inferencev1.DeploymentSpec
	}{
		{"empty project", " ", modelID, "alice", "qwen-small", validSpec},
		{"uppercase project", "Demo", modelID, "alice", "qwen-small", validSpec},
		{"project with underscore", "demo_project", modelID, "alice", "qwen-small", validSpec},
		{"project with dot", "demo.project", modelID, "alice", "qwen-small", validSpec},
		{"project with leading hyphen", "-demo", modelID, "alice", "qwen-small", validSpec},
		{"project with trailing hyphen", "demo-", modelID, "alice", "qwen-small", validSpec},
		{"project too long", strings.Repeat("a", 64), modelID, "alice", "qwen-small", validSpec},
		{"invalid model UUID", "project-1", "invalid", "alice", "qwen-small", validSpec},
		{"zero model UUID", "project-1", uuid.Nil.String(), "alice", "qwen-small", validSpec},
		{"missing username", "project-1", modelID, " ", "qwen-small", validSpec},
		{"empty deployment name", "project-1", modelID, "alice", " ", validSpec},
		{"uppercase deployment name", "project-1", modelID, "alice", "Qwen", validSpec},
		{"deployment name with underscore", "project-1", modelID, "alice", "qwen_small", validSpec},
		{"deployment name with dot", "project-1", modelID, "alice", "qwen.small", validSpec},
		{"deployment name with leading hyphen", "project-1", modelID, "alice", "-qwen", validSpec},
		{"deployment name with trailing hyphen", "project-1", modelID, "alice", "qwen-", validSpec},
		{"deployment name too long", "project-1", modelID, "alice", strings.Repeat("a", 64), validSpec},
		{"missing spec", "project-1", modelID, "alice", "qwen-small", nil},
		{"empty spec", "project-1", modelID, "alice", "qwen-small", &inferencev1.DeploymentSpec{}},
		{"unsupported provider", "project-1", modelID, "alice", "qwen-small", &inferencev1.DeploymentSpec{ProviderType: "OTHER", ProviderHuggingFace: &inferencev1.HuggingFaceProvider{Repo: "Qwen/model"}}},
		{"missing provider settings", "project-1", modelID, "alice", "qwen-small", &inferencev1.DeploymentSpec{ProviderType: "HUGGING_FACE"}},
		{"empty repository", "project-1", modelID, "alice", "qwen-small", &inferencev1.DeploymentSpec{ProviderType: "HUGGING_FACE", ProviderHuggingFace: &inferencev1.HuggingFaceProvider{Repo: " "}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := &inferencev1.StartDeploymentRequest{
				Project:        test.project,
				ModelId:        test.modelID,
				Username:       test.username,
				DeploymentName: test.deploymentName,
				Spec:           test.spec,
			}
			response, err := NewHandler(nil).StartDeployment(context.Background(), request)
			if response != nil || status.Code(err) != codes.InvalidArgument {
				t.Fatalf("expected InvalidArgument, got response=%v error=%v", response, err)
			}
		})
	}
	t.Run("nil request", func(t *testing.T) {
		_, err := NewHandler(nil).StartDeployment(context.Background(), nil)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})
}
