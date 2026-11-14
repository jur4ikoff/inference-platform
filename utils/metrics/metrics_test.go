package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsExposeHTTPAndGRPC(t *testing.T) {
	m := New()
	handler := m.InstrumentHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
	m.ObserveGRPC("/inference.v1.InferenceService/ListDeployments", "OK", time.Millisecond)

	response := httptest.NewRecorder()
	m.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", response.Code)
	}
	for _, expected := range []string{
		`app_http_requests_total{code="201",method="post"} 1`,
		`app_http_request_duration_seconds_count{method="post"} 1`,
		`app_grpc_requests_total{code="OK",method="/inference.v1.InferenceService/ListDeployments"} 1`,
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("missing metric %q", expected)
		}
	}
}
