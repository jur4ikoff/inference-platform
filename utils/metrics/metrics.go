// Package metrics exposes bounded-cardinality application metrics to Prometheus.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry     *prometheus.Registry
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	grpcRequests *prometheus.CounterVec
	grpcDuration *prometheus.HistogramVec
}

func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "app_http_requests_total", Help: "Number of HTTP requests.",
		}, []string{"method", "code"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "app_http_request_duration_seconds", Help: "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method"}),
		grpcRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "app_grpc_requests_total", Help: "Number of unary gRPC requests.",
		}, []string{"method", "code"}),
		grpcDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "app_grpc_request_duration_seconds", Help: "Unary gRPC request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method"}),
	}
	m.registry.MustRegister(m.httpRequests, m.httpDuration, m.grpcRequests, m.grpcDuration,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) Register(collector prometheus.Collector) error {
	return m.registry.Register(collector)
}

func (m *Metrics) InstrumentHTTP(next http.Handler) http.Handler {
	instrumented := promhttp.InstrumentHandlerCounter(m.httpRequests,
		promhttp.InstrumentHandlerDuration(m.httpDuration, next))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metrics", "/healthz", "/readyz":
			next.ServeHTTP(w, r)
		default:
			instrumented.ServeHTTP(w, r)
		}
	})
}

func (m *Metrics) ObserveGRPC(method, code string, duration time.Duration) {
	m.grpcRequests.WithLabelValues(method, code).Inc()
	m.grpcDuration.WithLabelValues(method).Observe(duration.Seconds())
}
