package manager

import (
	"errors"
	"fmt"
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
	if len(host)+2*(63+1) > 253 {
		return nil, errors.New("PROXY_PUBLIC_URL hostname is too long for deployment subdomains")
	}
	for _, label := range strings.Split(host, ".") {
		if !domain.ValidDNSLabel(label) {
			return nil, errors.New("PROXY_PUBLIC_URL must use a DNS hostname, for example localhost")
		}
	}
	// IP addresses cannot be used as the parent of deployment subdomains.
	if strings.Trim(host, "0123456789.") == "" {
		return nil, errors.New("PROXY_PUBLIC_URL must use a DNS hostname instead of an IP address")
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
			request.Out.Header.Del("X-Forwarded-Prefix")
		},
		ModifyResponse: func(response *http.Response) error {
			for name := range response.Header {
				if strings.HasPrefix(strings.ToLower(name), "access-control-") {
					response.Header.Del(name)
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
	origin := r.publicURL.Scheme + "://" + host
	if origins := request.Header.Values("Origin"); len(origins) != 0 && (len(origins) != 1 || origins[0] != origin) {
		http.Error(w, "cross-origin model requests are not allowed", http.StatusForbidden)
		return
	}
	if host == r.publicURL.Host {
		http.NotFound(w, request)
		return
	}
	// Only the configured suffix and port are accepted. Forwarded host headers are not trusted.
	selector, ok := strings.CutSuffix(host, "."+r.publicURL.Host)
	project, deploymentName, separated := strings.Cut(selector, ".")
	if !ok || !separated || !domain.ValidDNSLabel(project) || !domain.ValidDNSLabel(deploymentName) {
		http.NotFound(w, request)
		return
	}
	r.mu.RLock()
	backend, exists := r.routes[routeKey{project, deploymentName}]
	r.mu.RUnlock()
	if !exists || request.URL.Path == "/proxy" || strings.HasPrefix(request.URL.Path, "/proxy/") {
		http.NotFound(w, request)
		return
	}
	backend.handler.ServeHTTP(w, request)
}
