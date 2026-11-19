// Package config reads and validates the process environment.
// Loading .env files belongs to the local launcher (for example Docker Compose),
// not to the application binary.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment string
	ServiceName string
	Version     string

	HTTPAddr       string
	RequestTimeout time.Duration

	DatabaseURL      string
	DatabaseMaxConns int32
	DatabaseMinConns int32

	JWTPrivateKeyPath string
	JWTKeyID          string
	JWTIssuer         string
	JWTAudience       string
	AccessTokenTTL    time.Duration
	PasswordWorkers   int

	LogLevel  string
	Telemetry TelemetryConfig
}

// BootstrapConfig deliberately excludes HTTP, telemetry and JWT key material:
// the bootstrap binary never starts a server and never issues tokens.
type BootstrapConfig struct {
	DatabaseURL      string
	DatabaseMaxConns int32
	DatabaseMinConns int32
	PasswordWorkers  int
}

type TelemetryConfig struct {
	Enabled      bool
	OTLPEndpoint string
	Insecure     bool
	SampleRatio  float64
}

func Load() (Config, error) {
	environment := envOrDefault("APP_ENV", "local")
	if err := validateEnvironment(environment); err != nil {
		return Config{}, err
	}

	requestTimeout, err := positiveDuration("REQUEST_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	accessTokenTTL, err := positiveDuration("ACCESS_TOKEN_TTL", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}
	if accessTokenTTL > 30*time.Minute {
		return Config{}, errors.New("ACCESS_TOKEN_TTL must not exceed 30m")
	}
	passwordWorkers, err := boundedInt("PASSWORD_HASH_WORKERS", 2, 1, 8)
	if err != nil {
		return Config{}, err
	}
	maxConns, err := boundedInt("DB_MAX_CONNS", 10, 1, 100)
	if err != nil {
		return Config{}, err
	}
	minConns, err := boundedInt("DB_MIN_CONNS", 0, 0, maxConns)
	if err != nil {
		return Config{}, err
	}
	telemetry, err := loadTelemetry(environment)
	if err != nil {
		return Config{}, err
	}

	config := Config{
		Environment:       environment,
		ServiceName:       envOrDefault("OTEL_SERVICE_NAME", "user-service"),
		Version:           envOrDefault("APP_VERSION", "dev"),
		HTTPAddr:          envOrDefault("HTTP_ADDR", ":8080"),
		RequestTimeout:    requestTimeout,
		DatabaseURL:       requiredEnv("DATABASE_URL"),
		DatabaseMaxConns:  int32(maxConns),
		DatabaseMinConns:  int32(minConns),
		JWTPrivateKeyPath: requiredEnv("JWT_PRIVATE_KEY_PATH"),
		JWTKeyID:          requiredEnv("JWT_KEY_ID"),
		JWTIssuer:         envOrDefault("JWT_ISSUER", "user-service"),
		JWTAudience:       envOrDefault("JWT_AUDIENCE", "inference-platform"),
		AccessTokenTTL:    accessTokenTTL,
		PasswordWorkers:   passwordWorkers,
		LogLevel:          envOrDefault("LOG_LEVEL", defaultLogLevel(environment)),
		Telemetry:         telemetry,
	}

	if config.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if config.JWTPrivateKeyPath == "" || config.JWTKeyID == "" {
		return Config{}, errors.New("JWT_PRIVATE_KEY_PATH and JWT_KEY_ID are required")
	}
	return config, nil
}

func LoadBootstrap() (BootstrapConfig, error) {
	maxConns, err := boundedInt("DB_MAX_CONNS", 10, 1, 100)
	if err != nil {
		return BootstrapConfig{}, err
	}
	minConns, err := boundedInt("DB_MIN_CONNS", 0, 0, maxConns)
	if err != nil {
		return BootstrapConfig{}, err
	}
	workers, err := boundedInt("PASSWORD_HASH_WORKERS", 2, 1, 8)
	if err != nil {
		return BootstrapConfig{}, err
	}
	url := requiredEnv("DATABASE_URL")
	if url == "" {
		return BootstrapConfig{}, errors.New("DATABASE_URL is required")
	}
	return BootstrapConfig{DatabaseURL: url, DatabaseMaxConns: int32(maxConns), DatabaseMinConns: int32(minConns), PasswordWorkers: workers}, nil
}

func loadTelemetry(environment string) (TelemetryConfig, error) {
	enabled, err := optionalBool("OTEL_ENABLED", false)
	if err != nil {
		return TelemetryConfig{}, err
	}

	defaultRatio := 1.0
	if environment == "production" {
		defaultRatio = 0.1
	}
	sampleRatio, err := ratio("OTEL_TRACE_SAMPLE_RATIO", defaultRatio)
	if err != nil {
		return TelemetryConfig{}, err
	}

	insecure, err := optionalBool("OTEL_EXPORTER_OTLP_INSECURE", false)
	if err != nil {
		return TelemetryConfig{}, err
	}

	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if enabled && endpoint == "" {
		return TelemetryConfig{}, errors.New("OTEL_EXPORTER_OTLP_ENDPOINT is required when OTEL_ENABLED=true")
	}

	return TelemetryConfig{
		Enabled:      enabled,
		OTLPEndpoint: endpoint,
		Insecure:     insecure,
		SampleRatio:  sampleRatio,
	}, nil
}

func requiredEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func positiveDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", key, value)
	}
	return duration, nil
}

func boundedInt(key string, fallback, min, max int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	integer, err := strconv.Atoi(value)
	if err != nil || integer < min || integer > max {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, min, max)
	}
	return integer, nil
}

func optionalBool(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean, got %q", key, value)
	}
	return parsed, nil
}

func ratio(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || parsed > 1 {
		return 0, fmt.Errorf("%s must be a number from 0 to 1, got %q", key, value)
	}
	return parsed, nil
}

func validateEnvironment(environment string) error {
	switch environment {
	case "local", "development", "staging", "production":
		return nil
	default:
		return fmt.Errorf("APP_ENV must be local, development, staging, or production; got %q", environment)
	}
}

func defaultLogLevel(environment string) string {
	if environment == "local" || environment == "development" {
		return "debug"
	}
	return "info"
}
