package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jur4ikoff/inference-service/internal/api"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/manager"
	"github.com/jur4ikoff/inference-service/internal/repository"
	"github.com/jur4ikoff/inference-service/internal/runtime"
	"github.com/jur4ikoff/inference-service/internal/service"
	"github.com/jur4ikoff/utils/logging"
	"github.com/jur4ikoff/utils/postgres"
)

const (
	shutdownTimeout      = 15 * time.Second
	modelShutdownTimeout = 15 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "inference-service failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	routes, err := manager.NewProxyManager(cfg.Proxy)
	if err != nil {
		return fmt.Errorf("configure model proxy: %w", err)
	}

	logger, err := logging.NewLogger(
		cfg.Environment,
		cfg.LogLevel,
		cfg.ServiceName,
		cfg.Version,
	)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	ctx = logger.WithContext(ctx)

	var repo service.DeploymentRepository
	var monitorRepo manager.DeploymentRepository
	if cfg.DatabaseURL != "" {
		startupCtx, cancelStartup := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
		pool, err := postgres.NewPool(startupCtx, cfg.DatabaseURL, cfg.DatabaseMaxConns, cfg.DatabaseMinConns)
		if err != nil {
			cancelStartup()
			return fmt.Errorf("create PostgreSQL pool: %w", err)
		}

		deploymentRepo := repository.NewDeploymentRepository(pool)
		repo = deploymentRepo
		monitorRepo = deploymentRepo
		err = deploymentRepo.Check(startupCtx)
		cancelStartup()
		if err != nil {
			pool.Close()
			return fmt.Errorf("check PostgreSQL connection: %w", err)
		}
		defer pool.Close()
		logger.Info().Msg("PostgreSQL connection established")
	}

	runtimeClient, err := runtime.NewRuntimeService(cfg.RuntimeService)
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	defer func() {
		if err := runtimeClient.Close(); err != nil {
			logger.Error().Err(err).Msg("failed to close Docker client")
		}
	}()

	deploymentService := service.NewDeploymentService(repo, cfg.Service, runtimeClient)
	server := api.NewServer(deploymentService, cfg)

	if err = deploymentService.Init(ctx); err != nil {
		return fmt.Errorf("init deployment service: %w", err)
	}

	var deploymentManager *manager.DeploymentManager
	if monitorRepo != nil {
		deploymentManager, err = manager.NewDeploymentManager(monitorRepo, runtimeClient, routes, cfg.DeploymentManager)
		if err != nil {
			return fmt.Errorf("create deployment manager: %w", err)
		}
		if err := deploymentManager.Init(ctx); err != nil {
			return fmt.Errorf("initial deployment reconciliation: %w", err)
		}
	}

	listener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("open gRPC port: %w", err)
	}
	defer listener.Close()
	logger.Info().Str("address", listener.Addr().String()).Msg("GRPC server starting")
	httpServer := &http.Server{
		Addr: cfg.HTTPAddr, Handler: routes, ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}
	httpListener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("open HTTP proxy port: %w", err)
	}
	defer httpListener.Close()
	defer httpServer.Close()
	logger.Info().Str("address", httpListener.Addr().String()).Msg("HTTP model proxy starting")

	var stopDeploymentManager func()
	if deploymentManager != nil {
		managerCtx, stopManager := context.WithCancel(context.WithoutCancel(ctx))
		managerDone := make(chan struct{})
		stopDeploymentManager = func() {
			stopManager()
			<-managerDone
		}
		defer stopDeploymentManager()
		go func() {
			defer close(managerDone)
			if err := deploymentManager.Run(managerCtx); err != nil {
				logger.Error().Err(err).Msg("deployment manager stopped with an error")
			}
		}()
		logger.Info().Msg("deployment manager started")
	}

	serveErr := make(chan error, 2)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	go func() {
		err := httpServer.Serve(httpListener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErr <- err
	}()

	var serveFailure error
	select {
	case err := <-serveErr:
		if err != nil {
			serveFailure = fmt.Errorf("serve: %w", err)
			logger.Error().Err(err).Msg("server stopped unexpectedly")
		}
	case <-ctx.Done():
		stopSignals()
	}

	logger.Info().Dur("timeout", shutdownTimeout).Msg("shutting down servers")
	stopped := make(chan struct{}, 2)
	go func() {
		server.GracefulStop()
		stopped <- struct{}{}
	}()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()
	go func() {
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			_ = httpServer.Close()
		}
		stopped <- struct{}{}
	}()
	for count := 0; count < 2; {
		select {
		case <-stopped:
			count++
		case <-shutdownCtx.Done():
			logger.Warn().Msg("server shutdown timeout reached; forcing stop")
			server.Stop()
			_ = httpServer.Close()
			for count < 2 {
				<-stopped
				count++
			}
		}
	}
	logger.Info().Msg("servers stopped")
	if stopDeploymentManager != nil {
		stopDeploymentManager()
	}
	modelCtx, cancelModels := context.WithTimeout(context.WithoutCancel(ctx), modelShutdownTimeout)
	defer cancelModels()
	var modelErr error
	if deploymentManager != nil {
		modelErr = deploymentManager.Shutdown(modelCtx)
	} else {
		modelErr = runtimeClient.StopModelContainers(modelCtx)
	}
	if modelErr != nil {
		logger.Error().Err(modelErr).Msg("failed to shut down model containers")
	} else {
		logger.Info().Msg("model containers stopped")
	}
	return errors.Join(serveFailure, modelErr)
}
