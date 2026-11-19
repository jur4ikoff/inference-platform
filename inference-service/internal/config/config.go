package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment            string
	LogLevel               string
	ServiceName            string
	Version                string
	GRPCAddr               string
	HTTPAddr               string
	RequestTimeout         time.Duration
	DatabaseURL            string
	DatabaseMaxConns       int32
	DatabaseMinConns       int32
	DatabaseConnectTimeout time.Duration
	Service                ServiceConfig
	RuntimeService         RuntimeServiceConfig
	DeploymentManager      DeploymentManagerConfig
	Proxy                  ProxyConfig
}

type ProxyConfig struct {
	PublicURL string
}

type ServiceConfig struct {
	InferenceServerImage string
}

type RuntimeServiceConfig struct {
	PortHost      string
	DockerNetwork string
}

type DeploymentManagerConfig struct {
	SyncInterval      time.Duration
	OperationTimeout  time.Duration
	ReconnectInterval time.Duration
}

func Load() (Config, error) {
	environment := env("APP_ENV", "local")
	logLevel := "info"
	if environment == "local" || environment == "development" {
		logLevel = "debug"
	}

	maxConns, err := strconv.ParseInt(env("DB_MAX_CONNS", "10"), 10, 32)
	if err != nil || maxConns <= 0 {
		return Config{}, fmt.Errorf("DB_MAX_CONNS must be a positive integer")
	}
	minConns, err := strconv.ParseInt(env("DB_MIN_CONNS", "0"), 10, 32)
	if err != nil || minConns < 0 || minConns > maxConns {
		return Config{}, fmt.Errorf("DB_MIN_CONNS must be between 0 and DB_MAX_CONNS")
	}
	connectTimeout, err := time.ParseDuration(env("DB_CONNECT_TIMEOUT", "5s"))
	if err != nil || connectTimeout <= 0 {
		return Config{}, fmt.Errorf("DB_CONNECT_TIMEOUT must be a positive duration")
	}

	requestTimeout, err := time.ParseDuration(env("REQUEST_TIMEOUT", "90s"))
	if err != nil || requestTimeout <= 0 {
		return Config{}, fmt.Errorf("REQUEST_TIMEOUT must be a positive duration")
	}
	portHost, err := strconv.ParseUint(env("RUNTIME_PORT_HOST", "18081"), 10, 16)
	if err != nil || portHost == 0 {
		return Config{}, fmt.Errorf("RUNTIME_PORT_HOST must be an integer between 1 and 65535")
	}
	var managerConfig DeploymentManagerConfig
	for _, setting := range []struct {
		key      string
		fallback string
		target   *time.Duration
	}{
		{"DEPLOYMENT_MANAGER_SYNC_INTERVAL", "5s", &managerConfig.SyncInterval},
		{"DEPLOYMENT_MANAGER_OPERATION_TIMEOUT", "5s", &managerConfig.OperationTimeout},
		{"DEPLOYMENT_MANAGER_RECONNECT_INTERVAL", "1s", &managerConfig.ReconnectInterval},
	} {
		duration, err := time.ParseDuration(env(setting.key, setting.fallback))
		if err != nil || duration <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration", setting.key)
		}
		*setting.target = duration
	}
	return Config{
		Environment: environment, LogLevel: env("LOG_LEVEL", logLevel),
		ServiceName: env("OTEL_SERVICE_NAME", "inference-service"), Version: env("APP_VERSION", "dev"),
		GRPCAddr:               env("GRPC_ADDR", ":9091"),
		HTTPAddr:               env("HTTP_ADDR", ":8080"),
		RequestTimeout:         requestTimeout,
		DatabaseURL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
		DatabaseMaxConns:       int32(maxConns),
		DatabaseMinConns:       int32(minConns),
		DatabaseConnectTimeout: connectTimeout,
		Service: ServiceConfig{
			InferenceServerImage: env("INFERENCE_SERVER_IMAGE", "ghcr.io/ggml-org/llama.cpp:server"),
		},
		RuntimeService: RuntimeServiceConfig{
			PortHost:      strconv.FormatUint(portHost, 10),
			DockerNetwork: strings.TrimSpace(os.Getenv("DOCKER_NETWORK")),
		},
		DeploymentManager: managerConfig,
		Proxy: ProxyConfig{
			PublicURL: env("PROXY_PUBLIC_URL", "http://localhost:8082"),
		},
	}, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
