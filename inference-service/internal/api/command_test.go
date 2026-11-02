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
		UserId:         "0197eb5a-37c6-7e48-8cd6-0c19edcd72f2",
		DeploymentName: "qwen-small",
		Spec: &inferencev1.DeploymentSpec{
			ProviderType: "HUGGING_FACE",
			ProviderHuggingFace: &inferencev1.HuggingFaceProvider{
				Repo: "Qwen/model",
			},
		},
	}
}

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
	if command.Project != request.Project || command.UserID.String() != request.UserId || command.ModelID.String() != request.ModelId || command.DeploymentName != request.DeploymentName {
		t.Fatalf("request identifiers were not preserved: %+v", command)
	}
	if command.Spec.ProviderType != domain.ModelProviderTypeHuggingFace || command.Spec.ProviderHuggingFace == nil || command.Spec.ProviderHuggingFace.Repo != "Qwen/model" {
		t.Fatalf("provider specification was not decoded: %+v", command.Spec)
	}
}

func TestStartDeploymentRejectsInvalidCommandBeforeService(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*inferencev1.StartDeploymentRequest)
	}{
		{"empty project", func(r *inferencev1.StartDeploymentRequest) { r.Project = " " }},
		{"uppercase project", func(r *inferencev1.StartDeploymentRequest) { r.Project = "Demo" }},
		{"project with underscore", func(r *inferencev1.StartDeploymentRequest) { r.Project = "demo_project" }},
		{"project with dot", func(r *inferencev1.StartDeploymentRequest) { r.Project = "demo.project" }},
		{"project with leading hyphen", func(r *inferencev1.StartDeploymentRequest) { r.Project = "-demo" }},
		{"project with trailing hyphen", func(r *inferencev1.StartDeploymentRequest) { r.Project = "demo-" }},
		{"project too long", func(r *inferencev1.StartDeploymentRequest) { r.Project = strings.Repeat("a", 64) }},
		{"invalid model UUID", func(r *inferencev1.StartDeploymentRequest) { r.ModelId = "invalid" }},
		{"zero model UUID", func(r *inferencev1.StartDeploymentRequest) { r.ModelId = uuid.Nil.String() }},
		{"missing user UUID", func(r *inferencev1.StartDeploymentRequest) { r.UserId = "" }},
		{"invalid user UUID", func(r *inferencev1.StartDeploymentRequest) { r.UserId = "invalid" }},
		{"zero user UUID", func(r *inferencev1.StartDeploymentRequest) { r.UserId = uuid.Nil.String() }},
		{"empty deployment name", func(r *inferencev1.StartDeploymentRequest) { r.DeploymentName = " " }},
		{"uppercase deployment name", func(r *inferencev1.StartDeploymentRequest) { r.DeploymentName = "Qwen" }},
		{"deployment name with underscore", func(r *inferencev1.StartDeploymentRequest) { r.DeploymentName = "qwen_small" }},
		{"deployment name with dot", func(r *inferencev1.StartDeploymentRequest) { r.DeploymentName = "qwen.small" }},
		{"deployment name with leading hyphen", func(r *inferencev1.StartDeploymentRequest) { r.DeploymentName = "-qwen" }},
		{"deployment name with trailing hyphen", func(r *inferencev1.StartDeploymentRequest) { r.DeploymentName = "qwen-" }},
		{"deployment name too long", func(r *inferencev1.StartDeploymentRequest) { r.DeploymentName = strings.Repeat("a", 64) }},
		{"missing spec", func(r *inferencev1.StartDeploymentRequest) { r.Spec = nil }},
		{"empty spec", func(r *inferencev1.StartDeploymentRequest) { r.Spec = &inferencev1.DeploymentSpec{} }},
		{"unsupported provider", func(r *inferencev1.StartDeploymentRequest) { r.Spec.ProviderType = "OTHER" }},
		{"missing provider settings", func(r *inferencev1.StartDeploymentRequest) { r.Spec.ProviderHuggingFace = nil }},
		{"empty repository", func(r *inferencev1.StartDeploymentRequest) {
			r.Spec.ProviderHuggingFace.Repo = " "
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := validStartDeploymentRequest()
			test.change(request)
			// A nil service ensures invalid requests are rejected before any side effects.
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
