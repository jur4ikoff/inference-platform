package logging

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

func NewLogger(environment, level, serviceName, version string) (zerolog.Logger, error) {
	parsedLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.Logger{}, fmt.Errorf("parse log level: %w", err)
	}

	zerolog.TimeFieldFormat = time.RFC3339Nano
	var output io.Writer = os.Stdout

	if environment == "local" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	return zerolog.New(output).
		Level(parsedLevel).
		With().
		Timestamp().
		Str("service.name", serviceName).
		Str("service.version", version).
		Str("deployment.environment", environment).
		Logger(), nil
}
