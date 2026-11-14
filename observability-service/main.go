package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type probe struct{ name, network, address string }

type responseCounts struct {
	mu    sync.Mutex
	codes map[int]uint64
}

type countedResponse struct {
	http.ResponseWriter
	status int
}

func (w *countedResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	if status >= 200 && w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *countedResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (c *responseCounts) track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		response := &countedResponse{ResponseWriter: w}
		next.ServeHTTP(response, r)
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		c.mu.Lock()
		c.codes[status]++
		c.mu.Unlock()
	})
}

func (c *responseCounts) write(w http.ResponseWriter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = fmt.Fprint(w, "# HELP observability_http_responses_total Number of HTTP responses by status code.\n# TYPE observability_http_responses_total counter\n")
	for code, count := range c.codes {
		_, _ = fmt.Fprintf(w, "observability_http_responses_total{code=\"%d\"} %d\n", code, count)
	}
}

var probes = []probe{
	{"nginx", "http", "http://nginx/health"},
	{"user-service", "http", "http://user-service:8080/healthz"},
	{"model-service", "http", "http://model-service:8081/healthz"},
	{"inference-service", "http", "http://inference-service:8080/healthz"},
	{"adminer", "http", "http://adminer:8080/"},
	{"postgres", "tcp", "postgres:5432"},
	{"loki", "tcp", "loki:3100"},
	{"prometheus", "tcp", "prometheus:9090"},
	{"tempo", "tcp", "tempo:3200"},
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
	counts := &responseCounts{codes: make(map[int]uint64)}
	dockerMetrics := newDockerMetrics("/var/run/docker.sock")
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = fmt.Fprint(w, "# HELP observability_service_info Observability service is running.\n# TYPE observability_service_info gauge\nobservability_service_info 1\n")
		counts.write(w)
		dockerMetrics.write(r.Context(), w)
		_, _ = fmt.Fprintf(w, "# HELP go_goroutines Number of goroutines.\n# TYPE go_goroutines gauge\ngo_goroutines %d\n", runtime.NumGoroutine())
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		_, _ = fmt.Fprintf(w, "# HELP go_memstats_alloc_bytes Go heap allocated bytes.\n# TYPE go_memstats_alloc_bytes gauge\ngo_memstats_alloc_bytes %d\n", mem.Alloc)
		var usage syscall.Rusage
		if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) == nil {
			seconds := float64(usage.Utime.Sec+usage.Stime.Sec) + float64(usage.Utime.Usec+usage.Stime.Usec)/1e6
			_, _ = fmt.Fprintf(w, "# HELP process_cpu_seconds_total Total user and system CPU time.\n# TYPE process_cpu_seconds_total counter\nprocess_cpu_seconds_total %g\n", seconds)
		}
		if data, err := os.ReadFile("/proc/self/statm"); err == nil {
			fields := strings.Fields(string(data))
			if len(fields) > 1 {
				if pages, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					_, _ = fmt.Fprintf(w, "# HELP process_resident_memory_bytes Resident memory size in bytes.\n# TYPE process_resident_memory_bytes gauge\nprocess_resident_memory_bytes %d\n", pages*uint64(os.Getpagesize()))
				}
			}
		}
	})
	mux.HandleFunc("GET /healthz", report)
	mux.HandleFunc("GET /ready", readiness)
	server := &http.Server{Addr: ":8080", Handler: counts.track(mux), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("observability-service listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Printf("observability-service stopped: %v", err)
		os.Exit(1)
	}
}
