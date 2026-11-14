package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type dockerMetrics struct{ client *http.Client }

func newDockerMetrics(socket string) *dockerMetrics {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	return &dockerMetrics{client: &http.Client{Transport: transport}}
}

type dockerContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Labels map[string]string `json:"Labels"`
}

type dockerStats struct {
	CPU struct {
		Usage struct {
			Total uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
	} `json:"cpu_stats"`
	Memory struct {
		Usage uint64            `json:"usage"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
}

func (d *dockerMetrics) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Docker API returned %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}

func (d *dockerMetrics) write(parent context.Context, w io.Writer) {
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	_, _ = fmt.Fprint(w, "# HELP platform_container_cpu_usage_seconds_total CPU time consumed by a running container.\n# TYPE platform_container_cpu_usage_seconds_total counter\n# HELP platform_container_memory_working_set_bytes Container memory usage excluding inactive file cache.\n# TYPE platform_container_memory_working_set_bytes gauge\n# HELP platform_container_usage_scrape_success Whether all running container statistics were collected.\n# TYPE platform_container_usage_scrape_success gauge\n")
	var containers []dockerContainer
	if err := d.get(ctx, "/v1.41/containers/json", &containers); err != nil {
		_, _ = fmt.Fprintln(w, "platform_container_usage_scrape_success 0")
		return
	}
	type sample struct {
		item  dockerContainer
		stats dockerStats
		err   error
	}
	results := make(chan sample, len(containers))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, item := range containers {
		wg.Add(1)
		go func(item dockerContainer) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- sample{err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			var stats dockerStats
			err := d.get(ctx, "/v1.41/containers/"+url.PathEscape(item.ID)+"/stats?stream=false&one-shot=true", &stats)
			results <- sample{item: item, stats: stats, err: err}
		}(item)
	}
	wg.Wait()
	close(results)
	success := 1
	for s := range results {
		if s.err != nil {
			success = 0
			continue
		}
		name := s.item.ID
		if len(s.item.Names) > 0 {
			name = strings.TrimPrefix(s.item.Names[0], "/")
		}
		labels := fmt.Sprintf("container=%s,container_id=%s,compose_project=%s,compose_service=%s", strconv.Quote(name), strconv.Quote(s.item.ID), strconv.Quote(s.item.Labels["com.docker.compose.project"]), strconv.Quote(s.item.Labels["com.docker.compose.service"]))
		memory := s.stats.Memory.Usage
		inactive := s.stats.Memory.Stats["inactive_file"]
		if inactive == 0 {
			inactive = s.stats.Memory.Stats["total_inactive_file"]
		}
		if memory >= inactive {
			memory -= inactive
		}
		_, _ = fmt.Fprintf(w, "platform_container_cpu_usage_seconds_total{%s} %g\nplatform_container_memory_working_set_bytes{%s} %d\n", labels, float64(s.stats.CPU.Usage.Total)/1e9, labels, memory)
	}
	_, _ = fmt.Fprintf(w, "platform_container_usage_scrape_success %d\n", success)
}
