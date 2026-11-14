package transport

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryReads retries only known read-only RPCs; deployment creation is never retried here.
func RetryReads(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	switch method {
	case "/inference.v1.InferenceService/ListDeployments", "/inference.v1.InferenceService/ListAllDeployments", "/inference.v1.InferenceService/GetDeploymentByID", "/inference.v1.InferenceService/GetDeploymentByName", "/grpc.health.v1.Health/Check":
	default:
		return invoke(ctx, method, req, reply, cc, opts...)
	}
	const attempts = 3
	for attempt := 0; attempt < attempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := invoke(attemptCtx, method, req, reply, cc, opts...)
		cancel()
		if err == nil || ctx.Err() != nil || attempt == attempts-1 || (status.Code(err) != codes.Unavailable && status.Code(err) != codes.DeadlineExceeded) {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return status.FromContextError(ctx.Err()).Err()
		case <-timer.C:
		}
	}
	return nil
}
