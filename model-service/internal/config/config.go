package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	DatabaseMaxConn   int32
	DatabaseMinConn   int32
	RequestTimeout    time.Duration
	InferenceGRPCAddr string
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

	return Config{
		HTTPAddr:          envOrDefault("HTTP_ADDR", ":8081"),
		DatabaseURL:       databaseURL,
		DatabaseMaxConn:   maxConn,
		DatabaseMinConn:   minConn,
		RequestTimeout:    timeout,
		InferenceGRPCAddr: envOrDefault("INFERENCE_GRPC_ADDR", "localhost:9091"),
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
