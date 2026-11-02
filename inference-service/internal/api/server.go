package api

import (
	"context"

	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/service"
	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

func NewServer(inference *service.DeploymentService, config config.Config) *grpc.Server {
	server := grpc.NewServer(
		grpc.MaxRecvMsgSize(maxInputBytes+(64<<10)),
		grpc.MaxSendMsgSize(maxOutputBytes+(64<<10)),
		grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			ctx, cancel := context.WithTimeout(ctx, config.RequestTimeout)
			defer cancel()
			response, err := handler(ctx, request)
			if ctx.Err() != nil {
				return nil, status.FromContextError(ctx.Err()).Err()
			}
			return response, err
		}),
	)
	inferencev1.RegisterInferenceServiceServer(server, NewHandler(inference))
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthv1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(inferencev1.InferenceService_ServiceDesc.ServiceName, healthv1.HealthCheckResponse_SERVING)
	healthv1.RegisterHealthServer(server, healthServer)
	reflection.Register(server)

	return server
}
