package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestLifecycleThroughGRPC(t *testing.T) {
	models, repository, runtime := lifecycleService(t, nil)
	repository.model.Name = "renamed"
	state, err := models.Start(context.Background(), "project-1", repository.model.ID)
	if err != nil || state.ModelID != repository.model.ID || state.Name != "renamed" {
		t.Fatalf("create deployment: state=%+v error=%v", state, err)
	}
	assertRuntimeCall(t, runtime, "start", repository.model)
	if repository.project != "project-1" || repository.modelID != repository.model.ID {
		t.Fatalf("create deployment used the wrong selector: %+v", repository)
	}
}

func TestRuntimeErrorsThroughGRPC(t *testing.T) {
	for _, test := range []struct {
		code codes.Code
		err  error
	}{
		{codes.InvalidArgument, service.ErrInvalidInput},
		{codes.NotFound, domain.ErrModelNotFound},
		{codes.AlreadyExists, service.ErrAlreadyRunning},
		{codes.FailedPrecondition, service.ErrNotRunning},
		{codes.Unavailable, service.ErrRuntimeUnavailable},
		{codes.DeadlineExceeded, context.DeadlineExceeded},
		{codes.Canceled, context.Canceled},
	} {
		t.Run(test.code.String(), func(t *testing.T) {
			models, repository, runtime := lifecycleService(t, status.Error(test.code, "private runtime details"))
			if _, err := models.Start(context.Background(), "project-1", repository.model.ID); !errors.Is(err, test.err) {
				t.Fatalf("got %v, want %v", err, test.err)
			}
			assertRuntimeCall(t, runtime, "start", repository.model)
		})
	}
}

func TestLifecycleRejectsInvalidOrMissingModelBeforeRuntime(t *testing.T) {
	models, repository, runtime := lifecycleService(t, nil)
	for _, selector := range [][2]string{{" ", repository.model.ID}, {"project-1", " "}} {
		if _, err := models.Start(context.Background(), selector[0], selector[1]); !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("invalid selector: %v", err)
		}
	}
	if repository.calls != 0 {
		t.Fatal("invalid selector reached repository")
	}
	repository.err = domain.ErrModelNotFound
	if _, err := models.Start(context.Background(), "another-project", repository.model.ID); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("missing model: %v", err)
	}
	if repository.project != "another-project" || repository.modelID != repository.model.ID {
		t.Fatal("wrong selector used for missing model")
	}
	select {
	case call := <-runtime.calls:
		t.Fatalf("invalid or missing model reached runtime: %+v", call)
	default:
	}
}

func TestStartRejectsMalformedSpecBeforeGRPC(t *testing.T) {
	for _, spec := range []string{`{`, `null`, `[]`, `{"provider_type":42}`, `{"unknown_field":"value"}`} {
		t.Run(spec, func(t *testing.T) {
			models, repository, runtime := lifecycleService(t, nil)
			repository.model.Spec = json.RawMessage(spec)
			if _, err := models.Start(context.Background(), "project-1", repository.model.ID); !errors.Is(err, service.ErrInvalidInput) {
				t.Fatalf("expected invalid input for malformed spec, got %v", err)
			}
			select {
			case call := <-runtime.calls:
				t.Fatalf("malformed spec reached gRPC: %+v", call)
			default:
			}
		})
	}
}

func lifecycleService(t *testing.T, runtimeErr error) (*service.ModelService, *lifecycleRepository, *lifecycleRuntime) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	runtime := &lifecycleRuntime{calls: make(chan runtimeCall, 1), err: runtimeErr}
	server := grpc.NewServer()
	inferencev1.RegisterInferenceServiceServer(server, runtime)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	connection, err := grpc.NewClient("passthrough:///inference",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	repository := &lifecycleRepository{model: domain.Model{
		ID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", Project: "project-1", Name: "demo",
		UserID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f2",
		Spec:   json.RawMessage(`{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"Qwen/model"}}`),
	}}
	models := service.NewModelService(repository, service.NewGRPCModelRuntime(inferencev1.NewInferenceServiceClient(connection)))
	return models, repository, runtime
}

func assertRuntimeCall(t *testing.T, runtime *lifecycleRuntime, action string, model domain.Model) {
	t.Helper()
	select {
	case call := <-runtime.calls:
		if call.action != action || call.project != "project-1" || call.modelID != "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1" {
			t.Fatalf("unexpected gRPC call: %+v", call)
		}
		if action == "start" {
			var expectedSpec inferencev1.DeploymentSpec
			if err := protojson.Unmarshal(model.Spec, &expectedSpec); err != nil {
				t.Fatal(err)
			}
			if call.userID != model.UserID || call.deploymentName != model.Name || !proto.Equal(call.spec, &expectedSpec) {
				t.Fatalf("model details were not passed through gRPC: %+v", call)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("missing gRPC call")
	}
}

type lifecycleRepository struct {
	service.ModelRepository
	model            domain.Model
	err              error
	project, modelID string
	calls            int
}

func (r *lifecycleRepository) Get(_ context.Context, project, modelID string) (domain.Model, error) {
	r.project, r.modelID = project, modelID
	r.calls++
	return r.model, r.err
}

type runtimeCall struct {
	action, project, modelID string
	userID, deploymentName   string
	spec                     *inferencev1.DeploymentSpec
}

type lifecycleRuntime struct {
	inferencev1.UnimplementedInferenceServiceServer
	calls chan runtimeCall
	err   error
}

func (r *lifecycleRuntime) StartDeployment(_ context.Context, request *inferencev1.StartDeploymentRequest) (*inferencev1.StartDeploymentResponse, error) {
	r.calls <- runtimeCall{
		action: "start", project: request.GetProject(), modelID: request.GetModelId(),
		userID: request.GetUserId(), deploymentName: request.GetDeploymentName(), spec: request.GetSpec(),
	}
	return &inferencev1.StartDeploymentResponse{}, r.err
}
