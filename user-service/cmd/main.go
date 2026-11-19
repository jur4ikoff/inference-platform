package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/user-service/internal/api"
	"github.com/jur4ikoff/user-service/internal/api/handler"
	"github.com/jur4ikoff/user-service/internal/config"
	"github.com/jur4ikoff/user-service/internal/infrastructure"
	"github.com/jur4ikoff/user-service/internal/repository"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/utils/metrics"
	"github.com/jur4ikoff/utils/telemetry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "user-service failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger, err := infrastructure.NewLogger(
		cfg.Environment,
		cfg.LogLevel,
		cfg.ServiceName,
		cfg.Version,
	)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}

	shutdownTracing, err := infrastructure.InitTracing(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize tracing: %w", err)
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
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer pool.Close()
	if err := checkSchema(startupCtx, pool); err != nil {
		return fmt.Errorf("check PostgreSQL schema: %w", err)
	}

	logger.Info().
		Str("address", cfg.HTTPAddr).
		Int32("db_max_conns", cfg.DatabaseMaxConns).
		Bool("tracing_enabled", cfg.Telemetry.Enabled).
		Msg("user service configuration loaded")

	users := repository.NewUserRepository(pool)
	privateKey, err := infrastructure.LoadRSAPrivateKey(cfg.JWTPrivateKeyPath)
	if err != nil {
		return fmt.Errorf("load JWT private key: %w", err)
	}
	tokens, err := infrastructure.NewTokenManager(privateKey, cfg.JWTKeyID, cfg.JWTIssuer, cfg.JWTAudience)
	if err != nil {
		return fmt.Errorf("initialize JWT signer: %w", err)
	}
	userService := service.NewUserService(
		users,
		infrastructure.NewPasswordHasher(cfg.PasswordWorkers),
		tokens,
		infrastructure.IDGenerator{},
		infrastructure.Clock{},
		cfg.AccessTokenTTL,
	)
	projects := repository.NewProjectRepository(pool)
	router := api.NewRouter(
		handler.NewAuthHandler(userService, service.NewProjectTokenService(projects, tokens, cfg.AccessTokenTTL, infrastructure.Clock{})),
		handler.NewUserHandler(userService, service.NewPlatformRoleService(users)),
		handler.NewProjectHandler(service.NewProjectService(projects)),
		handler.NewRoleHandler(service.NewRoleService(repository.NewRoleRepository(pool))),
		cfg.Version,
	)
	var ready atomic.Bool
	ready.Store(true)
	root := http.NewServeMux()
	appMetrics := metrics.New()
	root.Handle("GET /metrics", appMetrics.Handler())
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	root.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		checkCtx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := pool.Ping(checkCtx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	root.Handle("/", appMetrics.InstrumentHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodPut {
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		}
		router.ServeHTTP(w, r)
	})))
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           telemetry.HTTPHandler(api.LogRequests(logger, http.TimeoutHandler(root, cfg.RequestTimeout, "request timed out")), "user-service"),
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
		logger.Info().Msg("user service shutting down")
		ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("shutdown HTTP server")
		}
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("run HTTP server: %w", err)
		}
	}
	return nil
}

func checkSchema(ctx context.Context, pool *pgxpool.Pool) error {
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('users') IS NOT NULL AND to_regclass('roles') IS NOT NULL AND to_regclass('projects') IS NOT NULL").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("required UserService tables are missing")
	}
	return nil
}
