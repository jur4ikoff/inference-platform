package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment       string
	LogLevel          string
	Version           string
	HTTPAddr          string
	DatabaseURL       string
	DatabaseMaxConn   int32
	DatabaseMinConn   int32
	RequestTimeout    time.Duration
	InferenceGRPCAddr string
	UserJWKSURL       string
	JWTIssuer         string
	JWTAudience       string
	JWKSCacheTTL      time.Duration
}

func Load() (Config, error) {
	timeout, err := time.ParseDuration(envOrDefault("REQUEST_TIMEOUT", "90s"))
	if err != nil || timeout <= 0 {
		return Config{}, fmt.Errorf("REQUEST_TIMEOUT must be a positive duration")
	}

	maxConn, err := int32Env("DB_MAX_CONNS", 10)
	if err != nil || maxConn <= 0 {
		return Config{}, fmt.Errorf("DB_MAX_CONNS must be a positive integer")
	}
	minConn, err := int32Env("DB_MIN_CONNS", 0)
	if err != nil || minConn < 0 || minConn > maxConn {
		return Config{}, fmt.Errorf("DB_MIN_CONNS must be between 0 and DB_MAX_CONNS")
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	cacheTTL, err := time.ParseDuration(envOrDefault("JWKS_CACHE_TTL", "1m"))
	if err != nil || cacheTTL <= 0 || cacheTTL > 5*time.Minute {
		return Config{}, fmt.Errorf("JWKS_CACHE_TTL must be between 0 and 5 minutes")
	}
	issuer, audience := strings.TrimSpace(os.Getenv("JWT_ISSUER")), strings.TrimSpace(os.Getenv("JWT_AUDIENCE"))
	if issuer == "" || audience == "" {
		return Config{}, fmt.Errorf("JWT_ISSUER and JWT_AUDIENCE are required")
	}

	return Config{
		Environment:       envOrDefault("APP_ENV", "production"),
		LogLevel:          envOrDefault("LOG_LEVEL", "info"),
		Version:           envOrDefault("APP_VERSION", "dev"),
		HTTPAddr:          envOrDefault("HTTP_ADDR", ":8081"),
		DatabaseURL:       databaseURL,
		DatabaseMaxConn:   maxConn,
		DatabaseMinConn:   minConn,
		RequestTimeout:    timeout,
		InferenceGRPCAddr: envOrDefault("INFERENCE_GRPC_ADDR", "localhost:9091"),
		UserJWKSURL:       envOrDefault("USER_JWKS_URL", "http://user-service:8080/.well-known/jwks.json"),
		JWTIssuer:         issuer,
		JWTAudience:       audience,
		JWKSCacheTTL:      cacheTTL,
	}, nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func int32Env(key string, fallback int32) (int32, error) {
	value := envOrDefault(key, strconv.Itoa(int(fallback)))
	parsed, err := strconv.ParseInt(value, 10, 32)
	return int32(parsed), err
}
