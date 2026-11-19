package metrics

import (
	"context"
	"errors"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/prometheus/client_golang/prometheus"
)

type limitsDocker struct{ fail bool }

func (d limitsDocker) ListServiceContainers(context.Context, string) (client.ContainerListResult, error) {
	if d.fail {
		return client.ContainerListResult{}, errors.New("docker unavailable")
	}
	return client.ContainerListResult{Items: []container.Summary{
		{ID: "one", Labels: map[string]string{"com.docker.compose.service": "model-service"}},
		{ID: "two", Labels: map[string]string{"com.docker.compose.service": "user-service"}},
	}}, nil
}
func (d limitsDocker) GetContainer(_ context.Context, id string) (client.ContainerInspectResult, error) {
	cfg := &container.HostConfig{}
	if id == "one" {
		cfg.NanoCPUs = 1_500_000_000
		cfg.Memory = 512 << 20
	}
	return client.ContainerInspectResult{Container: container.InspectResponse{Name: "/" + id, HostConfig: cfg}}, nil
}

func collectLimitMetrics(t *allure.Context, fail bool) map[string][]float64 {
	t.Helper()
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewContainerLimitsCollector(limitsDocker{fail: fail}, "test"))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	values := map[string][]float64{}
	for _, f := range families {
		for _, m := range f.Metric {
			values[f.GetName()] = append(values[f.GetName()], m.Gauge.GetValue())
		}
	}
	return values
}

// Техника: разбиение на классы эквивалентности; варианты заданных и незаданных ограничений контейнеров.
func TestContainerLimitsExportsConfiguredResources(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		values := collectLimitMetrics(allureCtx, false)
		if values["platform_container_limits_scrape_success"][0] != 1 || len(values["platform_container_cpu_limit_cores"]) != 1 || values["platform_container_cpu_limit_cores"][0] != 1.5 || values["platform_container_memory_limit_bytes"][0] != 512<<20 || len(values["platform_container_limit_configured"]) != 4 {
			allureCtx.Fatalf("unexpected limits: %v", values)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки доступа к Docker.
func TestContainerLimitsReportsDockerFailure(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		values := collectLimitMetrics(allureCtx, true)
		if values["platform_container_limits_scrape_success"][0] != 0 || len(values["platform_container_cpu_limit_cores"]) != 0 {
			allureCtx.Fatalf("unexpected failure metrics: %v", values)
		}
	})
}
