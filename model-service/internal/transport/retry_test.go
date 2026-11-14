package transport

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetryReadsOnly(t *testing.T) {
	for _, tc := range []struct {
		method         string
		failures, want int
	}{
		{"/inference.v1.InferenceService/ListDeployments", 2, 3},
		{"/inference.v1.InferenceService/StartDeployment", 2, 1},
		{"/inference.v1.InferenceService/ListDeployments", 5, 3},
	} {
		calls := 0
		invoke := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			calls++
			if tc.method != "/inference.v1.InferenceService/StartDeployment" {
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 2*time.Second {
					t.Error("read attempt has no bounded timeout")
				}
			}
			if calls <= tc.failures {
				return status.Error(codes.Unavailable, "temporarily unavailable")
			}
			return nil
		}
		err := RetryReads(context.Background(), tc.method, nil, nil, nil, invoke)
		if calls != tc.want || (tc.failures < tc.want) != (err == nil) {
			t.Fatalf("method=%s calls=%d err=%v", tc.method, calls, err)
		}
	}
}

func TestRetryReadsDoesNotRetryPermanentErrorsOrCancellation(t *testing.T) {
	calls := 0
	invoke := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		return status.Error(codes.InvalidArgument, "invalid request")
	}
	err := RetryReads(context.Background(), "/inference.v1.InferenceService/ListDeployments", nil, nil, nil, invoke)
	if calls != 1 || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("calls=%d err=%v", calls, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	calls = 0
	invoke = func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		cancel()
		return status.Error(codes.Unavailable, "unavailable")
	}
	_ = RetryReads(ctx, "/inference.v1.InferenceService/ListDeployments", nil, nil, nil, invoke)
	if calls != 1 {
		t.Fatalf("canceled request made %d attempts", calls)
	}
}
