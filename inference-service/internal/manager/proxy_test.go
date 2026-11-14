package manager

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Техника: разбиение на классы эквивалентности маршрутов, host и origin.
func TestProxyDeploymentPaths(t *testing.T) {
	proxy, err := NewProxyManager(config.ProxyConfig{PublicURL: "http://localhost:8082"})
	if err != nil {
		t.Fatal(err)
	}
	first := domain.DeploymentState{ID: uuid.New(), Project: "ml", DeploymentName: "qwen"}
	second := domain.DeploymentState{ID: uuid.New(), Project: "test", DeploymentName: "qwen"}
	for _, deployment := range []domain.DeploymentState{first, second} {
		if err := proxy.Upsert(deployment, "http://model:8080"); err != nil {
			t.Fatal(err)
		}
		key := routeKey{deployment.Project, deployment.DeploymentName}
		proxy.routes[key].handler.(*httputil.ReverseProxy).Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got := req.Header.Get("X-Forwarded-Prefix"); got != "/"+deployment.Project+"/"+deployment.DeploymentName {
				t.Errorf("prefix=%s", got)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(req.URL.RequestURI()))}, nil
		})
	}
	for _, tc := range []struct {
		path, host, origin, body string
		status                   int
	}{
		{"/ml/qwen/v1/chat/completions?x=a%2Fb", "localhost:8082", "http://localhost:8082", "/v1/chat/completions?x=a%2Fb", 200},
		{"/test/qwen/health", "localhost:8082", "", "/health", 200},
		{"/other/qwen/", "localhost:8082", "", "", 404},
		{"/ml/qwen/", "evil:8082", "", "", 404},
		{"/ml/qwen/", "localhost:8082", "http://evil", "", 403},
		{"/ml/qwen", "localhost:8082", "", "", 308},
	} {
		req := httptest.NewRequest(http.MethodGet, "http://"+tc.host+tc.path, nil)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		proxy.ServeHTTP(w, req)
		if w.Code != tc.status || tc.body != "" && w.Body.String() != tc.body {
			t.Errorf("%s: status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}

// Техника: разбиение на классы эквивалентности; случай HTML-ответа с ресурсами.
func TestPrepareUIResponse(t *testing.T) {
	body := `<html><head><script src="/assets/app.js"></script></head><body></body></html>`
	response := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
	if err := prepareUIResponse(response, "/ml/qwen/"); err != nil {
		t.Fatal(err)
	}
	result, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "inference:") || !strings.Contains(string(result), `src="/ml/qwen/assets/app.js"`) || response.ContentLength != int64(len(result)) {
		t.Fatalf("unexpected HTML: %s", result)
	}
}

// Техника: разбиение на классы эквивалентности и анализ граничных значений некорректных адресов.
func TestProxyRejectsInvalidPublicURL(t *testing.T) {
	for _, address := range []string{"", "http://localhost:0", "http://localhost:65536", "http://localhost/path", "https://user:password@models.example.com", "http://localhost?x=y", "http://localhost?", "ftp://localhost", "http://bad_host", "http://-bad.localhost"} {
		if _, err := NewProxyManager(config.ProxyConfig{PublicURL: address}); err == nil {
			t.Errorf("accepted %q", address)
		}
	}
}

// Техника: разбиение на классы эквивалентности корректных адресов.
func TestProxyAcceptsValidPublicURL(t *testing.T) {
	for _, address := range []string{"http://127.0.0.1:8082", "https://models.example.com:443/"} {
		if _, err := NewProxyManager(config.ProxyConfig{PublicURL: address}); err != nil {
			t.Errorf("rejected %q: %v", address, err)
		}
	}
}

// Техника: разбиение на классы эквивалентности; случай сжатого HTML-ответа.
func TestProxyDecompressesGzipUI(t *testing.T) {
	proxy, err := NewProxyManager(config.ProxyConfig{PublicURL: "http://localhost:8082"})
	if err != nil {
		t.Fatal(err)
	}
	deployment := domain.DeploymentState{ID: uuid.New(), Project: "ml", DeploymentName: "qwen"}
	if err := proxy.Upsert(deployment, "http://model:8080"); err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			req, err := http.ReadRequest(bufio.NewReader(server))
			if err != nil {
				return
			}
			if req.Header.Get("Accept-Encoding") != "gzip" {
				fmt.Fprint(server, "HTTP/1.1 415 Unsupported Media Type\r\nContent-Length: 0\r\n\r\n")
				return
			}
			var body bytes.Buffer
			gz := gzip.NewWriter(&body)
			_, _ = gz.Write([]byte("<html><head></head><body>model</body></html>"))
			_ = gz.Close()
			fmt.Fprintf(server, "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", body.Len())
			_, _ = server.Write(body.Bytes())
		}()
		return client, nil
	}}
	defer transport.CloseIdleConnections()
	proxy.routes[routeKey{"ml", "qwen"}].handler.(*httputil.ReverseProxy).Transport = transport
	req := httptest.NewRequest("GET", "http://localhost:8082/ml/qwen/", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, req)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "inference:") || !strings.Contains(response.Body.String(), "model") || response.Header().Get("Content-Encoding") != "" {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

// Техника: разбиение на классы эквивалентности разрешённых и запрещённых хостов.
func TestProxyAllowsConfiguredAndLocalHosts(t *testing.T) {
	proxy, err := NewProxyManager(config.ProxyConfig{PublicURL: "http://192.168.50.139:8082"})
	if err != nil {
		t.Fatal(err)
	}
	deployment := domain.DeploymentState{ID: uuid.New(), Project: "ml", DeploymentName: "qwen"}
	if err := proxy.Upsert(deployment, "http://model:8080"); err != nil {
		t.Fatal(err)
	}
	proxy.routes[routeKey{"ml", "qwen"}].handler.(*httputil.ReverseProxy).Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	for _, tc := range []struct {
		host, origin string
		status       int
	}{
		{"192.168.50.139:8082", "http://192.168.50.139:8082", 200},
		{"localhost:8082", "http://localhost:8082", 200},
		{"127.0.0.1:8082", "http://127.0.0.1:8082", 200},
		{"localhost:8082", "http://192.168.50.139:8082", 403},
		{"localhost:8083", "", 404}, {"evil.example:8082", "", 404},
	} {
		req := httptest.NewRequest("GET", "http://"+tc.host+"/ml/qwen/health", nil)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, req)
		if response.Code != tc.status {
			t.Errorf("%s origin=%s: got %d, want %d", tc.host, tc.origin, response.Code, tc.status)
		}
	}
}

func proxyWithRoute(t *testing.T) (*ProxyManager, domain.DeploymentState) {
	t.Helper()
	proxy, err := NewProxyManager(config.ProxyConfig{PublicURL: "http://localhost:8082"})
	if err != nil {
		t.Fatal(err)
	}
	state := domain.DeploymentState{ID: uuid.New(), Project: "demo", DeploymentName: "model"}
	if err := proxy.Upsert(state, "http://model:8080"); err != nil {
		t.Fatal(err)
	}
	return proxy, state
}

// Техника: разбиение на классы эквивалентности; случай корректного endpoint.
func TestProxyUpsertAcceptsHTTPEndpoint(t *testing.T) {
	proxy, err := NewProxyManager(config.ProxyConfig{PublicURL: "http://localhost:8082"})
	if err != nil {
		t.Fatal(err)
	}
	state := domain.DeploymentState{ID: uuid.New(), Project: "demo", DeploymentName: "model"}
	if err := proxy.Upsert(state, "http://model:8080"); err != nil {
		t.Fatal(err)
	}
	if _, exists := proxy.keys[state.ID]; !exists {
		t.Fatal("route was not registered")
	}
}

// Техника: разбиение на классы эквивалентности; случай endpoint с неподдерживаемой схемой.
func TestProxyUpsertRejectsInvalidEndpoint(t *testing.T) {
	proxy, err := NewProxyManager(config.ProxyConfig{PublicURL: "http://localhost:8082"})
	if err != nil {
		t.Fatal(err)
	}
	state := domain.DeploymentState{ID: uuid.New(), Project: "demo", DeploymentName: "model"}
	if err := proxy.Upsert(state, "file:///secret"); err == nil {
		t.Fatal("invalid endpoint accepted")
	}
	if _, exists := proxy.keys[state.ID]; exists {
		t.Fatal("invalid route was registered")
	}
}

// Техника: разбиение на классы эквивалентности; случай deployment, присутствующего в актуальном списке.
func TestProxyPruneKeepsKnownDeployment(t *testing.T) {
	proxy, state := proxyWithRoute(t)
	proxy.Prune([]domain.DeploymentState{state})
	if _, exists := proxy.keys[state.ID]; !exists {
		t.Fatal("known route was removed")
	}
}

// Техника: разбиение на классы эквивалентности; случай deployment, отсутствующего в актуальном списке.
func TestProxyPruneRemovesMissingDeployment(t *testing.T) {
	proxy, state := proxyWithRoute(t)
	proxy.Prune(nil)
	if _, exists := proxy.keys[state.ID]; exists {
		t.Fatal("missing route was retained")
	}
}

// Техника: разбиение на классы эквивалентности; случай существующего ключа.
func TestProxyRemoveDeletesExistingRoute(t *testing.T) {
	proxy, state := proxyWithRoute(t)
	proxy.Remove(state.ID)
	if _, exists := proxy.keys[state.ID]; exists {
		t.Fatal("route was retained")
	}
}

// Техника: разбиение на классы эквивалентности; случай отсутствующего ключа.
func TestProxyRemoveIgnoresUnknownID(t *testing.T) {
	proxy, state := proxyWithRoute(t)
	proxy.Remove(uuid.New())
	if _, exists := proxy.keys[state.ID]; !exists {
		t.Fatal("unrelated route was removed")
	}
}
