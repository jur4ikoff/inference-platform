package api

import (
	"context"
	"strings"
	"time"

	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/service"
	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"github.com/jur4ikoff/utils/metrics"
	"github.com/jur4ikoff/utils/telemetry"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

func NewServer(inference *service.DeploymentService, config config.Config, appMetrics *metrics.Metrics, loggers ...zerolog.Logger) *grpc.Server {
	logger := zerolog.Nop()
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	server := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.MaxRecvMsgSize(maxInputBytes+(64<<10)),
		grpc.MaxSendMsgSize(maxOutputBytes+(64<<10)),
		grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			started := time.Now()
			requestLogger := logger
			if md, ok := metadata.FromIncomingContext(ctx); ok {
				fields := logger.With()
				if values := md.Get("x-log-username"); len(values) == 1 {
					fields = fields.Str("username", values[0])
				}
				if values := md.Get("x-log-project"); len(values) == 1 {
					fields = fields.Str("project", values[0])
				}
				requestLogger = fields.Logger()
			}
			ctx = telemetry.WithLogger(ctx, requestLogger)
			ctx, cancel := context.WithTimeout(ctx, config.RequestTimeout)
			defer cancel()
			response, err := handler(ctx, request)
			if ctx.Err() != nil {
				err = status.FromContextError(ctx.Err()).Err()
				response = nil
			}
			if err != nil && !strings.HasPrefix(info.FullMethod, "/grpc.health.v1.Health/") {
				log.Ctx(ctx).Error().Err(err).Str("method", info.FullMethod).Msg("gRPC request failed")
			}
			if appMetrics != nil && !strings.HasPrefix(info.FullMethod, "/grpc.health.v1.Health/") {
				appMetrics.ObserveGRPC(info.FullMethod, status.Code(err).String(), time.Since(started))
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
