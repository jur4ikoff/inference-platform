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
	"github.com/jur4ikoff/model-service/internal/config"
	"github.com/jur4ikoff/model-service/internal/repository"
	"github.com/jur4ikoff/model-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		slog.Error("parse database configuration", "error", err)
		os.Exit(1)
	}
	poolConfig.MaxConns = cfg.DatabaseMaxConn
	poolConfig.MinConns = cfg.DatabaseMinConn
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		slog.Error("connect to PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		slog.Error("ping PostgreSQL", "error", err)
		os.Exit(1)
	}

	connection, err := grpc.NewClient(cfg.InferenceGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize((10<<20)+1024)),
	)
	if err != nil {
		slog.Error("create inference client", "error", err)
		os.Exit(1)
	}
	defer connection.Close()
	models := service.NewModelService(
		repository.NewModelRepository(pool),
		service.NewGRPCModelRuntime(inferencev1.NewInferenceServiceClient(connection)),
	)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           http.TimeoutHandler(api.NewHandler(models).Router(), cfg.RequestTimeout, `{"error":"request timed out"}`),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.RequestTimeout,
		WriteTimeout:      cfg.RequestTimeout,
		IdleTimeout:       60 * time.Second,
	}

	errorsChannel := make(chan error, 1)
	go func() {
		slog.Info("model service started", "address", cfg.HTTPAddr)
		errorsChannel <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown server", "error", err)
		}
	case err := <-errorsChannel:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("run server", "error", err)
		}
	}
}
