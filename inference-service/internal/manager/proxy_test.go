package manager

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
)

func TestProxyDeploymentOrigins(t *testing.T) {
	proxy, err := NewProxyManager(config.ProxyConfig{PublicURL: "http://localhost:8082"})
	if err != nil {
		t.Fatal(err)
	}
	first := domain.DeploymentState{ID: uuid.New(), Project: "demo", DeploymentName: "qwen"}
	second := domain.DeploymentState{ID: uuid.New(), Project: "demo", DeploymentName: "qwen-2"}
	otherProject := domain.DeploymentState{ID: uuid.New(), Project: "other", DeploymentName: "qwen"}
	for _, deployment := range []domain.DeploymentState{first, second, otherProject} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Content-Type", "text/html")
			body, _ := io.ReadAll(r.Body)
			fmt.Fprintf(w, "%s.%s|%s|%s|%s|%s", deployment.Project, deployment.DeploymentName, r.URL.RequestURI(), r.Method, body, r.Header.Get("X-Forwarded-Prefix"))
		}))
		t.Cleanup(upstream.Close)
		if err := proxy.Upsert(deployment, upstream.URL); err != nil {
			t.Fatal(err)
		}
	}
	firstHost := "demo.qwen.localhost:8082"
	secondHost := "demo.qwen-2.localhost:8082"
	for _, test := range []struct {
		name, host, path, method, origin, body string
		status                                 int
	}{
		{name: "old UI removed", host: "localhost:8082", path: "/proxy/demo/qwen/?a=b%2Fc", status: 404},
		{name: "old UI without trailing slash removed", host: "localhost:8082", path: "/proxy/demo/qwen", status: 404},
		{name: "old nested UI removed", host: "localhost:8082", path: "/proxy/demo/qwen/chat/123", status: 404},
		{name: "first UI", host: firstHost, path: "/", status: 200, body: "demo.qwen|/|GET||"},
		{name: "second UI", host: secondHost, path: "/", status: 200, body: "demo.qwen-2|/|GET||"},
		{name: "same name in another project", host: "other.qwen.localhost:8082", path: "/", status: 200, body: "other.qwen|/|GET||"},
		{name: "relative static asset", host: firstHost, path: "/_app/bundle.js", status: 200, body: "demo.qwen|/_app/bundle.js|GET||"},
		{name: "same-origin API", host: firstHost, path: "/v1/chat/completions?stream=true", method: "POST", origin: "http://" + firstHost, status: 200, body: "demo.qwen|/v1/chat/completions?stream=true|POST|request-body|"},
		{name: "old API removed", host: "localhost:8082", path: "/proxy/demo/qwen/v1/chat/completions", method: "POST", status: 404},
		{name: "cross-deployment API", host: secondHost, path: "/v1/chat/completions", method: "POST", origin: "http://" + firstHost, status: 403},
		{name: "cross-deployment preflight", host: secondHost, path: "/v1/chat/completions", method: "OPTIONS", origin: "http://" + firstHost, status: 403},
		{name: "cross-origin shared API", host: "localhost:8082", path: "/proxy/demo/qwen/v1/models", origin: "http://" + secondHost, status: 403},
		{name: "opaque origin", host: firstHost, path: "/v1/models", origin: "null", status: 403},
		{name: "another deployment path on first host", host: firstHost, path: "/proxy/demo/qwen-2/", status: 404},
		{name: "old UUID host removed", host: first.ID.String() + ".localhost:8082", path: "/", status: 404},
		{name: "unknown deployment", host: "demo.unknown.localhost:8082", path: "/", status: 404},
		{name: "unknown project", host: "unknown.qwen.localhost:8082", path: "/", status: 404},
		{name: "missing project", host: "qwen.localhost:8082", path: "/", status: 404},
		{name: "extra label", host: "demo.extra.qwen.localhost:8082", path: "/", status: 404},
		{name: "proxy root removed on deployment host", host: firstHost, path: "/proxy", status: 404},
		{name: "unknown base host", host: "evil.example:8082", path: "/proxy/demo/qwen/", status: 404},
		{name: "wrong port", host: "demo.qwen.localhost:8083", path: "/", status: 404},
		{name: "suffix confusion", host: "demo.qwen.localhost.evil.example:8082", path: "/", status: 404},
		{name: "missing route", host: "localhost:8082", path: "/proxy/demo/unknown/", status: 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			var body io.Reader
			if method != http.MethodGet {
				body = strings.NewReader("request-body")
			}
			request := httptest.NewRequest(method, "http://"+test.host+test.path, body)
			request.Header.Set("X-Forwarded-Host", secondHost)
			request.Header.Set("X-Forwarded-Prefix", "/proxy/demo/qwen-2")
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			response := httptest.NewRecorder()
			proxy.ServeHTTP(response, request)
			if response.Code != test.status || response.Header().Get("Location") != "" {
				t.Fatalf("status=%d location=%q, want %d without redirect", response.Code, response.Header().Get("Location"), test.status)
			}
			if test.body != "" && response.Body.String() != test.body {
				t.Fatalf("body=%q, want %q", response.Body.String(), test.body)
			}
			if response.Header().Get("Access-Control-Allow-Origin") != "" || response.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Fatal("upstream CORS must not expose a deployment to other origins")
			}
			if response.Code == http.StatusOK && strings.Contains(response.Header().Get("Content-Security-Policy"), "sandbox") {
				t.Fatal("the isolated UI must be able to execute its scripts")
			}
		})
	}

	proxy.Remove(first.ID)
	for _, address := range []string{"http://" + firstHost + "/", "http://localhost:8082/proxy/demo/qwen/"} {
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, address, nil))
		if response.Code != http.StatusNotFound {
			t.Fatal("removing a route must disable its deployment hostname")
		}
	}
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://other.qwen.localhost:8082/", nil))
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Body.String(), "other.qwen|") {
		t.Fatal("removing a deployment must not affect the same deployment name in another project")
	}
}

func TestProxyPublicURL(t *testing.T) {
	for _, address := range []string{"", "http://127.0.0.1:8082", "http://[::1]:8082", "http://localhost:0", "http://localhost:65536", "http://localhost/path", "https://user:password@models.example.com", "http://localhost?x=y", "http://localhost?", "ftp://localhost", "http://bad_host", "http://-bad.localhost"} {
		if _, err := NewProxyManager(config.ProxyConfig{PublicURL: address}); err == nil {
			t.Errorf("accepted invalid public origin %q", address)
		}
	}
	proxy, err := NewProxyManager(config.ProxyConfig{PublicURL: "https://models.example.com:443/"})
	if err != nil {
		t.Fatal(err)
	}
	if got := proxy.publicURL.String(); got != "https://models.example.com" {
		t.Fatalf("public origin=%q", got)
	}
}
