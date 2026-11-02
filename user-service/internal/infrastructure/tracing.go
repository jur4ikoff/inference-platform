package infrastructure

import (
	"context"
	"fmt"

	"github.com/jur4ikoff/user-service/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func InitTracing(ctx context.Context, cfg config.Config) (func(context.Context) error, error) {
	resource, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.version", cfg.Version),
			attribute.String("deployment.environment.name", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create OpenTelemetry resource: %w", err)
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource),
	}

	if cfg.Telemetry.Enabled {
		// The OTLP exporter uses the standard OTEL_EXPORTER_OTLP_* variables.
		// The configuration package validates the endpoint before this point.
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
		}

		options = append(options,
			sdktrace.WithSampler(sdktrace.ParentBased(
				sdktrace.TraceIDRatioBased(cfg.Telemetry.SampleRatio),
			)),
			sdktrace.WithBatcher(exporter),
		)
	} else {
		options = append(options, sdktrace.WithSampler(sdktrace.NeverSample()))
	}

	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider.Shutdown, nil
}
