package telemetry

import (
	"context"
	"net/http"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func Init(ctx context.Context, serviceName, version, environment string) (func(context.Context) error, error) {
	enabled, err := strconv.ParseBool(defaultEnv("OTEL_ENABLED", "false"))
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", serviceName), attribute.String("service.version", version), attribute.String("deployment.environment.name", environment)))
	if err != nil {
		return nil, err
	}
	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if enabled {
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, sdktrace.WithBatcher(exporter), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())))
	} else {
		opts = append(opts, sdktrace.WithSampler(sdktrace.NeverSample()))
	}
	provider := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return provider.Shutdown, nil
}

func defaultEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// WithLogger adds the current span identifiers to request logs.
func WithLogger(ctx context.Context, logger zerolog.Logger) context.Context {
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		logger = logger.With().Str("trace_id", span.TraceID().String()).Str("span_id", span.SpanID().String()).Logger()
	}
	return logger.WithContext(ctx)
}

func HTTPHandler(handler http.Handler, name string) http.Handler {
	return otelhttp.NewHandler(handler, name)
}

// QueryTracer traces PostgreSQL calls without recording query text or parameters.
type QueryTracer struct{}

func (QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	ctx, _ = otel.Tracer("postgresql").Start(ctx, "postgresql query", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("db.system.name", "postgresql")))
	return ctx
}

func (QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span := trace.SpanFromContext(ctx)
	if data.Err != nil {
		span.RecordError(data.Err)
		span.SetStatus(codes.Error, "database query failed")
	}
	span.End()
}
