package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestHTTPAndDatabaseSpansShareTrace(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		exporter := tracetest.NewInMemoryExporter()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
		otel.SetTracerProvider(provider)
		otel.SetTextMapPropagator(propagation.TraceContext{})
		t.Cleanup(func() { otel.SetTracerProvider(previousProvider); otel.SetTextMapPropagator(previousPropagator) })

		handler := HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tracer := QueryTracer{}
			ctx := tracer.TraceQueryStart(r.Context(), nil, pgx.TraceQueryStartData{})
			tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
			w.WriteHeader(http.StatusNoContent)
		}), "request")
		req := httptest.NewRequest(http.MethodGet, "/models", nil)
		req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
		handler.ServeHTTP(httptest.NewRecorder(), req)
		spans := exporter.GetSpans()
		if len(spans) != 2 {
			allureCtx.Fatalf("expected HTTP and database spans, got %d", len(spans))
		}
		if spans[0].Name != "postgresql query" || spans[0].Parent.SpanID() != spans[1].SpanContext.SpanID() || spans[1].SpanContext.TraceID().String() != "0123456789abcdef0123456789abcdef" {
			allureCtx.Fatalf("unexpected trace relationship: %+v", spans)
		}
	})
}
