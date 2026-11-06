package manager

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/rs/zerolog/log"
)

//go:embed proxy_ui.js
var proxyUIScript string

type routeKey struct{ project, deploymentName string }

type route struct {
	id       uuid.UUID
	endpoint string
	handler  http.Handler
}

type ProxyManager struct {
	mu        sync.RWMutex
	routes    map[routeKey]route
	keys      map[uuid.UUID]routeKey
	publicURL url.URL
}

func NewProxyManager(cfg config.ProxyConfig) (*ProxyManager, error) {
	publicURL, err := url.Parse(cfg.PublicURL)
	if err != nil || publicURL.Host == "" || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || publicURL.User != nil || (publicURL.Path != "" && publicURL.Path != "/") || publicURL.RawQuery != "" || publicURL.ForceQuery || publicURL.Fragment != "" {
		return nil, errors.New("PROXY_PUBLIC_URL must be an http(s) origin, for example http://localhost:8082")
	}
	host := strings.ToLower(publicURL.Hostname())
	for _, label := range strings.Split(host, ".") {
		if !domain.ValidDNSLabel(label) {
			return nil, errors.New("PROXY_PUBLIC_URL must use a DNS hostname, for example localhost")
		}
	}
	port := publicURL.Port()
	if port != "" {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return nil, errors.New("PROXY_PUBLIC_URL port must be between 1 and 65535")
		}
	}
	if publicURL.Scheme == "http" && port == "80" || publicURL.Scheme == "https" && port == "443" {
		port = ""
	}
	publicURL.Host = host
	if port != "" {
		publicURL.Host += ":" + port
	}
	publicURL.Path = ""
	return &ProxyManager{routes: make(map[routeKey]route), keys: make(map[uuid.UUID]routeKey), publicURL: *publicURL}, nil
}

func (r *ProxyManager) Upsert(deployment domain.DeploymentState, endpoint string) error {
	if deployment.ID == uuid.Nil {
		return errors.New("deployment ID must not be empty")
	}
	if !domain.ValidDNSLabel(deployment.Project) || !domain.ValidDNSLabel(deployment.DeploymentName) {
		return errors.New("project and deployment_name must be lowercase DNS labels of at most 63 characters")
	}
	target, err := url.Parse(endpoint)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return errors.New("invalid model endpoint")
	}
	key := routeKey{deployment.Project, deployment.DeploymentName}
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.routes[key]; ok {
		if current.id != deployment.ID {
			return fmt.Errorf("proxy route already exists: %s.%s", key.project, key.deploymentName)
		}
		if current.endpoint == endpoint {
			return nil
		}
	}
	if oldKey, ok := r.keys[deployment.ID]; ok && oldKey != key {
		delete(r.routes, oldKey)
	}
	backend := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.SetXForwarded()
			request.Out.Header.Set("X-Forwarded-Prefix", "/"+key.project+"/"+key.deploymentName)
			// Let Go request and transparently decompress gzip for UI rewriting.
			request.Out.Header.Del("Accept-Encoding")
			request.Out.Header.Del("If-None-Match")
			request.Out.Header.Del("If-Modified-Since")
		},
		ModifyResponse: func(response *http.Response) error {
			for name := range response.Header {
				if strings.HasPrefix(strings.ToLower(name), "access-control-") {
					response.Header.Del(name)
				}
			}
			if strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/html") && response.StatusCode == http.StatusOK && response.Request.Method != http.MethodHead {
				externalPrefix := ""
				if response.Request.Header.Get("X-Inference-Proxy-Prefix") == "/inference" {
					externalPrefix = "/inference"
				}
				if err := prepareUIResponse(response, externalPrefix+"/"+key.project+"/"+key.deploymentName+"/"); err != nil {
					return err
				}
			}
			response.Header.Set("Cross-Origin-Resource-Policy", "same-origin")
			response.Header.Set("X-Content-Type-Options", "nosniff")
			return nil
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, request *http.Request, err error) {
			if request.Context().Err() != nil {
				return
			}
			log.Ctx(request.Context()).Error().Err(err).Msg("model proxy request failed")
			http.Error(w, "model backend unavailable", http.StatusBadGateway)
		},
	}
	r.routes[key] = route{id: deployment.ID, endpoint: endpoint, handler: backend}
	r.keys[deployment.ID] = key
	return nil
}

func (r *ProxyManager) Remove(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if key, ok := r.keys[id]; ok {
		delete(r.routes, key)
		delete(r.keys, id)
	}
}

func (r *ProxyManager) Prune(states []domain.DeploymentState) {
	known := make(map[uuid.UUID]bool, len(states))
	for _, state := range states {
		known[state.ID] = true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, key := range r.keys {
		if !known[id] {
			delete(r.routes, key)
			delete(r.keys, id)
		}
	}
}

func (r *ProxyManager) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	host := strings.ToLower(request.Host)
	if r.publicURL.Scheme == "http" {
		host = strings.TrimSuffix(host, ":80")
	} else {
		host = strings.TrimSuffix(host, ":443")
	}
	localhost := "localhost"
	loopback := "127.0.0.1"
	if port := r.publicURL.Port(); port != "" {
		localhost += ":" + port
		loopback += ":" + port
	}
	if host != r.publicURL.Host && host != localhost && host != loopback {
		http.NotFound(w, request)
		return
	}
	if origins := request.Header.Values("Origin"); len(origins) != 0 && (len(origins) != 1 || origins[0] != r.publicURL.Scheme+"://"+host) {
		http.Error(w, "cross-origin model requests are not allowed", http.StatusForbidden)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(request.URL.Path, "/"), "/", 3)
	if len(parts) < 2 || !domain.ValidDNSLabel(parts[0]) || !domain.ValidDNSLabel(parts[1]) {
		http.NotFound(w, request)
		return
	}
	key := routeKey{parts[0], parts[1]}
	r.mu.RLock()
	backend, exists := r.routes[key]
	r.mu.RUnlock()
	if !exists {
		http.NotFound(w, request)
		return
	}
	prefix := "/" + key.project + "/" + key.deploymentName
	if len(parts) == 2 {
		location := prefix + "/"
		if request.URL.RawQuery != "" {
			location += "?" + request.URL.RawQuery
		}
		http.Redirect(w, request, location, http.StatusPermanentRedirect)
		return
	}
	http.StripPrefix(prefix, backend.handler).ServeHTTP(w, request)
}

func prepareUIResponse(response *http.Response, prefix string) error {
	if response.Header.Get("Content-Security-Policy") != "" || response.Header.Get("Content-Security-Policy-Report-Only") != "" {
		return errors.New("model UI content security policy prevents safe bootstrap injection")
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return fmt.Errorf("unsupported UI content encoding: %s", encoding)
	}
	const maxUIBytes = 20 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxUIBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxUIBytes {
		return errors.New("model UI exceeds size limit")
	}
	_ = response.Body.Close()
	// The bootstrap must run before the UI opens IndexedDB or issues API calls.
	tag := []byte("<script>window.__modelProxyPrefix=" + strconv.Quote(prefix) + ";" + proxyUIScript + "</script>")
	lower := bytes.ToLower(body)
	head := bytes.Index(lower, []byte("<head>"))
	if head < 0 {
		return errors.New("model UI has no head element")
	}
	head += len("<head>")
	body = append(append(append([]byte{}, body[:head]...), tag...), body[head:]...)
	// Root-relative static assets must stay under the deployment prefix.
	body = bytes.ReplaceAll(body, []byte(`src="/`), []byte(`src="`+prefix))
	body = bytes.ReplaceAll(body, []byte(`href="/`), []byte(`href="`+prefix))
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	response.Header.Set("Content-Length", strconv.Itoa(len(body)))
	response.Header.Del("ETag")
	response.Header.Del("Last-Modified")
	response.Header.Set("Cache-Control", "no-store")
	return nil
}
