package service

import (
	"context"
	"errors"
	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

type historyClient struct {
	inferencev1.InferenceServiceClient
	calls int
}

func (c *historyClient) ListAllDeployments(_ context.Context, req *inferencev1.ListAllDeploymentsRequest, _ ...grpc.CallOption) (*inferencev1.ListAllDeploymentsResponse, error) {
	c.calls++
	switch req.GetNextPageToken() {
	case "":
		return &inferencev1.ListAllDeploymentsResponse{Deployments: []*inferencev1.Deployment{{DeploymentId: "first", ModelId: "target", Project: "demo", Username: "alice", Status: inferencev1.ModelStatus_MODEL_STATUS_STOPPED, StartedAt: timestamppb.New(time.Unix(100, 0)), UpdatedAt: timestamppb.New(time.Unix(200, 0))}, {DeploymentId: "foreign", Project: "other"}}, NextPageToken: "100"}, nil
	case "100":
		return &inferencev1.ListAllDeploymentsResponse{Deployments: []*inferencev1.Deployment{{DeploymentId: "second", Project: "demo", Status: inferencev1.ModelStatus_MODEL_STATUS_FAILED}}}, nil
	default:
		panic("unexpected page token")
	}
}

// Техника: классы эквивалентности проектов и переходы между страницами.
func TestGetDeploymentsHistoryFiltersProjectAcrossPages(t *testing.T) {
	client := &historyClient{}
	result, err := NewGRPCModelRuntime(client).GetDeploymentsHistory(context.Background(), "demo")
	if err != nil || client.calls != 2 || len(result) != 2 {
		t.Fatalf("history=%+v calls=%d err=%v", result, client.calls, err)
	}
	if result[0].ID != "first" || result[0].Username != "alice" || result[0].Status != "stopped" || result[0].StartedAt == nil || result[0].StartedAt.Unix() != 100 || result[0].UpdatedAt == nil || result[0].UpdatedAt.Unix() != 200 || result[1].ID != "second" || result[1].Status != "failed" {
		t.Fatalf("unexpected history: %+v", result)
	}
}

func (c *historyClient) ListDeployments(_ context.Context, req *inferencev1.ListDeploymentsRequest, _ ...grpc.CallOption) (*inferencev1.ListDeploymentsResponse, error) {
	c.calls++
	switch req.GetNextPageToken() {
	case "":
		return &inferencev1.ListDeploymentsResponse{Deployments: []*inferencev1.Deployment{{DeploymentId: "first", Project: "demo", Username: "alice", Status: inferencev1.ModelStatus_MODEL_STATUS_STARTING}, {DeploymentId: "foreign", Project: "other"}}, NextPageToken: "100"}, nil
	case "100":
		return &inferencev1.ListDeploymentsResponse{Deployments: []*inferencev1.Deployment{{DeploymentId: "second", Project: "demo", Status: inferencev1.ModelStatus_MODEL_STATUS_RUNNING}}}, nil
	default:
		panic("unexpected page token")
	}
}

// Техника: классы эквивалентности проектов и переходы между страницами.
func TestGetActiveDeploymentsFiltersProjectAcrossPages(t *testing.T) {
	client := &historyClient{}
	result, err := NewGRPCModelRuntime(client).GetActiveDeployments(context.Background(), "demo")
	if err != nil || client.calls != 2 || len(result) != 2 {
		t.Fatalf("deployments=%+v calls=%d err=%v", result, client.calls, err)
	}
	if result[0].ID != "first" || result[0].Username != "alice" || result[0].Status != "starting" || result[1].ID != "second" || result[1].Status != "running" {
		t.Fatalf("unexpected deployments: %+v", result)
	}
}

// Техника: классы эквивалентности идентификаторов модели и проекта.
func TestGetDeploymentUsesModelIDAndProject(t *testing.T) {
	for _, tc := range []struct {
		project string
		want    error
	}{{"demo", nil}, {"other", ErrDeploymentNotFound}} {
		t.Run(tc.project, func(t *testing.T) {
			runtime := NewGRPCModelRuntime(&historyClient{})
			result, err := runtime.GetDeployment(context.Background(), domain.Model{ID: "target", Project: tc.project, Name: "name-without-container-suffix"})
			if !errors.Is(err, tc.want) || (err == nil && result.ID != "first") {
				t.Fatalf("deployment=%+v err=%v", result, err)
			}
		})
	}
}

type runtimeClient struct {
	inferencev1.InferenceServiceClient
	err      error
	username string
}

func (c *runtimeClient) StartDeployment(_ context.Context, request *inferencev1.StartDeploymentRequest, _ ...grpc.CallOption) (*inferencev1.StartDeploymentResponse, error) {
	c.username = request.GetUsername()
	if c.err != nil {
		return nil, c.err
	}
	return &inferencev1.StartDeploymentResponse{State: &inferencev1.ModelState{DeploymentId: "id"}}, nil
}

func TestCreateDeploymentUsesInitiatingUser(t *testing.T) {
	client := &runtimeClient{}
	runtime := NewGRPCModelRuntime(client)
	model := testutil.ModelMother{}.HuggingFace()
	model.Username = "model-owner"
	if _, err := runtime.CreateDeployment(WithActor(context.Background(), "initiator"), model); err != nil {
		t.Fatal(err)
	}
	if client.username != "initiator" {
		t.Fatalf("deployment username = %q", client.username)
	}
}
func (c *runtimeClient) StopDeployment(_ context.Context, _ *inferencev1.StopDeploymentRequest, _ ...grpc.CallOption) (*inferencev1.StopDeploymentResponse, error) {
	return &inferencev1.StopDeploymentResponse{}, c.err
}
func (c *runtimeClient) ListDeployments(context.Context, *inferencev1.ListDeploymentsRequest, ...grpc.CallOption) (*inferencev1.ListDeploymentsResponse, error) {
	return nil, c.err
}
func (c *runtimeClient) ListAllDeployments(context.Context, *inferencev1.ListAllDeploymentsRequest, ...grpc.CallOption) (*inferencev1.ListAllDeploymentsResponse, error) {
	return nil, c.err
}

// Техника: классы эквивалентности успеха и ошибки RPC.
func TestRuntimeCalls(t *testing.T) {
	for _, method := range []string{"start", "create", "stop", "history", "active", "get"} {
		for _, fail := range []bool{false, true} {
			if !fail && (method == "history" || method == "active" || method == "get") {
				continue
			}
			t.Run(method+map[bool]string{true: "/failure", false: "/success"}[fail], func(t *testing.T) {
				client := &runtimeClient{}
				if fail {
					client.err = status.Error(codes.Unavailable, "down")
				}
				r := NewGRPCModelRuntime(client)
				model := testutil.ModelMother{}.HuggingFace()
				var err error
				switch method {
				case "start":
					err = r.Start(context.Background(), model)
				case "create":
					_, err = r.CreateDeployment(context.Background(), model)
				case "stop":
					err = r.StopDeployment(context.Background(), domain.Deployment{ID: "id"})
				case "history":
					_, err = r.GetDeploymentsHistory(context.Background(), "demo")
				case "active":
					_, err = r.GetActiveDeployments(context.Background(), "demo")
				case "get":
					_, err = r.GetDeployment(context.Background(), model)
				}
				if fail && !errors.Is(err, ErrRuntimeUnavailable) || !fail && err != nil {
					t.Fatalf("err=%v", err)
				}
			})
		}
	}
}

type repeatedPageClient struct {
	inferencev1.InferenceServiceClient
}

func (*repeatedPageClient) ListAllDeployments(context.Context, *inferencev1.ListAllDeploymentsRequest, ...grpc.CallOption) (*inferencev1.ListAllDeploymentsResponse, error) {
	return &inferencev1.ListAllDeploymentsResponse{NextPageToken: "1"}, nil
}
func (*repeatedPageClient) ListDeployments(context.Context, *inferencev1.ListDeploymentsRequest, ...grpc.CallOption) (*inferencev1.ListDeploymentsResponse, error) {
	return &inferencev1.ListDeploymentsResponse{NextPageToken: "1"}, nil
}

// Техника: переходы между страницами с повторяющимся токеном.
func TestRuntimeRepeatedPageToken(t *testing.T) {
	for _, active := range []bool{true, false} {
		t.Run(map[bool]string{true: "active", false: "history"}[active], func(t *testing.T) {
			r := NewGRPCModelRuntime(&repeatedPageClient{})
			var err error
			if active {
				_, err = r.GetActiveDeployments(context.Background(), "demo")
			} else {
				_, err = r.GetDeploymentsHistory(context.Background(), "demo")
			}
			if err == nil {
				t.Fatal("repeated pagination token accepted")
			}
		})
	}
}
