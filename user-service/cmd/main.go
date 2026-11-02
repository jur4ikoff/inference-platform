package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jur4ikoff/user-service/internal/api"
	"github.com/jur4ikoff/user-service/internal/api/handler"
	"github.com/jur4ikoff/user-service/internal/config"
	"github.com/jur4ikoff/user-service/internal/infrastructure"
	"github.com/jur4ikoff/user-service/internal/repository"
	"github.com/jur4ikoff/user-service/internal/service"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "user-service startup failed: load configuration: %v\n", err)
		os.Exit(1)
	}

	logger, err := infrastructure.NewLogger(
		cfg.Environment,
		cfg.LogLevel,
		cfg.ServiceName,
		cfg.Version,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "user-service startup failed: create logger: %v\n", err)
		os.Exit(1)
	}

	shutdownTracing, err := infrastructure.InitTracing(ctx, cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("initialize tracing")
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("flush tracing data")
		}
	}()

	startupCtx, cancelStartup := context.WithTimeout(ctx, 10*time.Second)
	defer cancelStartup()

	pool, err := infrastructure.NewPool(
		startupCtx,
		cfg.DatabaseURL,
		cfg.DatabaseMaxConns,
		cfg.DatabaseMinConns,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect to PostgreSQL")
	}
	defer pool.Close()

	logger.Info().
		Str("address", cfg.HTTPAddr).
		Int32("db_max_conns", cfg.DatabaseMaxConns).
		Bool("tracing_enabled", cfg.Telemetry.Enabled).
		Msg("user service configuration loaded")

	users := repository.NewUserRepository(pool)
	userService := service.NewUserService(
		users,
		infrastructure.NewPasswordHasher(cfg.BcryptCost),
		infrastructure.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer),
		infrastructure.IDGenerator{},
		infrastructure.Clock{},
		cfg.AccessTokenTTL,
	)
	router := api.NewRouter(
		handler.NewAuthHandler(userService),
		handler.NewUserHandler(userService),
		handler.NewProjectHandler(service.NewProjectService(repository.NewProjectRepository(pool))),
		handler.NewRoleHandler(service.NewRoleService(repository.NewRoleRepository(pool))),
		cfg.Version,
	)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           http.TimeoutHandler(router, cfg.RequestTimeout, "request timed out"),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.RequestTimeout,
		WriteTimeout:      cfg.RequestTimeout,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info().Str("address", cfg.HTTPAddr).Msg("HTTP server started")
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("shutdown HTTP server")
		}
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("run HTTP server")
		}
	}
}
