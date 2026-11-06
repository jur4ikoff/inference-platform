package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

type probe struct{ name, network, address string }

var probes = []probe{
	{"nginx", "http", "http://nginx/health"},
	{"user-service", "http", "http://user-service:8080/healthz"},
	{"model-service", "http", "http://model-service:8081/healthz"},
	{"inference-service", "http", "http://inference-service:8080/healthz"},
	{"adminer", "http", "http://adminer:8080/"},
	{"postgres", "tcp", "postgres:5432"},
	{"loki", "tcp", "loki:3100"},
	{"tempo", "tcp", "tempo:3200"},
	{"otel-collector", "tcp", "otel-collector:4318"},
	{"alloy", "tcp", "alloy:12345"},
	{"grafana", "tcp", "grafana:3000"},
}

// User and model readiness checks include their dependencies. Inference exposes
// a liveness endpoint, so its entry checks HTTP availability only.
var readinessProbes = []probe{
	{"user-service", "http", "http://user-service:8080/readyz"},
	{"model-service", "http", "http://model-service:8081/readyz"},
	{"inference-service", "http", "http://inference-service:8080/healthz"},
}

func check(ctx context.Context, p probe) bool {
	if p.network == "tcp" {
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", p.address)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.address, nil)
	if err != nil {
		return false
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 400
}

func report(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	statuses := make(map[string]string, len(probes)+1)
	statuses["observability-service"] = "up"
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := "down"
			if check(ctx, p) {
				status = "up"
			}
			mu.Lock()
			statuses[p.name] = status
			mu.Unlock()
		}()
	}
	wg.Wait()
	overall := "up"
	for _, status := range statuses {
		if status != "up" {
			overall = "degraded"
			break
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if overall != "up" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(struct {
		Status   string            `json:"status"`
		Services map[string]string `json:"services"`
	}{overall, statuses})
}

func readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	checks := make(map[string]string, len(readinessProbes))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range readinessProbes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := "not_ready"
			if check(ctx, p) {
				result = "ready"
			}
			mu.Lock()
			checks[p.name] = result
			mu.Unlock()
		}()
	}
	wg.Wait()
	state := "ready"
	for _, result := range checks {
		if result != "ready" {
			state = "not_ready"
			break
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if state != "ready" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}{state, checks})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", report)
	mux.HandleFunc("GET /ready", readiness)
	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if server.ListenAndServe() != nil {
		os.Exit(1)
	}
}
