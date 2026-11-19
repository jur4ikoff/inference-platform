package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	inferencev1 "github.com/jur4ikoff/internal-api/gen/go/inference/v1"
	"github.com/jur4ikoff/model-service/internal/api"
	"github.com/jur4ikoff/model-service/internal/auth"
	"github.com/jur4ikoff/model-service/internal/config"
	"github.com/jur4ikoff/model-service/internal/idempotency"
	"github.com/jur4ikoff/model-service/internal/repository"
	"github.com/jur4ikoff/model-service/internal/service"
	"github.com/jur4ikoff/model-service/internal/transport"
	"github.com/jur4ikoff/utils/logging"
	"github.com/jur4ikoff/utils/metrics"
	"github.com/jur4ikoff/utils/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}
	logger, err := logging.NewLogger(cfg.Environment, cfg.LogLevel, "model-service", cfg.Version)
	if err != nil {
		slog.Error("create logger", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTracing, err := telemetry.Init(ctx, "model-service", cfg.Version, cfg.Environment)
	if err != nil {
		logger.Error().Err(err).Msg("initialize tracing")
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("shutdown tracing")
		}
	}()

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Error().Err(err).Msg("parse database configuration")
		os.Exit(1)
	}
	poolConfig.MaxConns = cfg.DatabaseMaxConn
	poolConfig.MinConns = cfg.DatabaseMinConn
	poolConfig.ConnConfig.Tracer = telemetry.QueryTracer{}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		logger.Error().Err(err).Msg("connect to PostgreSQL")
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		logger.Error().Err(err).Msg("ping PostgreSQL")
		os.Exit(1)
	}
	idempotencyConfig := poolConfig.Copy()
	idempotencyConfig.MaxConns = 4
	idempotencyConfig.MinConns = 0
	idempotencyPool, err := pgxpool.NewWithConfig(ctx, idempotencyConfig)
	if err != nil {
		logger.Error().Err(err).Msg("create idempotency database pool")
		os.Exit(1)
	}
	defer idempotencyPool.Close()
	connection, err := grpc.NewClient(cfg.InferenceGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithConnectParams(grpc.ConnectParams{Backoff: backoff.Config{BaseDelay: 100 * time.Millisecond, Multiplier: 1.6, Jitter: 0.2, MaxDelay: time.Second}, MinConnectTimeout: time.Second}),
		grpc.WithChainUnaryInterceptor(transport.RetryReads),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize((10<<20)+1024)),
	)
	if err != nil {
		logger.Error().Err(err).Msg("create inference client")
		os.Exit(1)
	}
	defer connection.Close()
	modelRepo := repository.NewModelRepository(pool)
	models := service.NewModelService(
		modelRepo,
		service.NewGRPCModelRuntime(inferencev1.NewInferenceServiceClient(connection)),
	)
	verifier, err := auth.NewJWKSVerifier(cfg.UserJWKSURL, cfg.JWTIssuer, cfg.JWTAudience, &http.Client{Timeout: 3 * time.Second, Transport: transport.RetryHTTPReads{Next: otelhttp.NewTransport(http.DefaultTransport)}}, cfg.JWKSCacheTTL)
	if err != nil {
		logger.Error().Err(err).Msg("configure JWT verifier")
		os.Exit(1)
	}
	root := http.NewServeMux()
	appMetrics := metrics.New()
	root.Handle("GET /metrics", appMetrics.Handler())
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	root.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := pool.Ping(checkCtx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		result, err := healthv1.NewHealthClient(connection).Check(checkCtx, &healthv1.HealthCheckRequest{Service: "inference.v1.InferenceService"})
		if err != nil || result.GetStatus() != healthv1.HealthCheckResponse_SERVING {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router := api.NewHandler(models, verifier).WithIdempotencyStore(idempotency.New(idempotencyPool)).Router()
	root.Handle("/", router)
	var handler http.Handler = root
	handler = appMetrics.InstrumentHTTP(handler)
	next := handler
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(telemetry.WithLogger(r.Context(), logger)))
	})
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           telemetry.HTTPHandler(http.TimeoutHandler(handler, cfg.RequestTimeout, `{"error":"request timed out"}`), "model-service"),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.RequestTimeout,
		WriteTimeout:      cfg.RequestTimeout,
		IdleTimeout:       60 * time.Second,
	}

	errorsChannel := make(chan error, 1)
	go func() {
		logger.Info().Str("address", cfg.HTTPAddr).Msg("model service started")
		errorsChannel <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info().Msg("model service shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("shutdown server")
		}
	case err := <-errorsChannel:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("run server")
		}
	}
}
