package metrics

import (
	"context"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/prometheus/client_golang/prometheus"
)

type serviceContainers interface {
	ListServiceContainers(context.Context, string) (client.ContainerListResult, error)
	GetContainer(context.Context, string) (client.ContainerInspectResult, error)
}

type ContainerLimitsCollector struct {
	docker                           serviceContainers
	project                          string
	cpu, memory, configured, success *prometheus.Desc
}

func NewContainerLimitsCollector(docker serviceContainers, project string) *ContainerLimitsCollector {
	labels := []string{"compose_project", "compose_service", "container"}
	return &ContainerLimitsCollector{docker: docker, project: project,
		cpu:        prometheus.NewDesc("platform_container_cpu_limit_cores", "Docker CPU quota in cores, omitted when unlimited.", labels, nil),
		memory:     prometheus.NewDesc("platform_container_memory_limit_bytes", "Docker memory limit in bytes, omitted when unlimited.", labels, nil),
		configured: prometheus.NewDesc("platform_container_limit_configured", "Whether the Docker container has a limit for the resource.", []string{"compose_project", "compose_service", "container", "resource"}, nil),
		success:    prometheus.NewDesc("platform_container_limits_scrape_success", "Whether Docker container limits were collected successfully.", nil, nil),
	}
}

func (c *ContainerLimitsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.cpu
	ch <- c.memory
	ch <- c.configured
	ch <- c.success
}

func (c *ContainerLimitsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var samples []prometheus.Metric
	seen := map[string]bool{}
	for _, project := range strings.Split(c.project, ",") {
		project = strings.TrimSpace(project)
		if project == "" || seen[project] {
			continue
		}
		seen[project] = true
		list, err := c.docker.ListServiceContainers(ctx, project)
		if err != nil {
			ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 0)
			return
		}
		for _, item := range list.Items {
			service := item.Labels["com.docker.compose.service"]
			if service == "" {
				continue
			}
			info, err := c.docker.GetContainer(ctx, item.ID)
			if errdefs.IsNotFound(err) {
				continue
			}
			if err != nil || info.Container.HostConfig == nil {
				ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 0)
				return
			}
			cfg := info.Container.HostConfig
			name := strings.TrimPrefix(info.Container.Name, "/")
			cpu := float64(cfg.NanoCPUs) / 1e9
			if cpu <= 0 && cfg.CPUQuota > 0 && cfg.CPUPeriod > 0 {
				cpu = float64(cfg.CPUQuota) / float64(cfg.CPUPeriod)
			}
			cpuSet, memorySet := 0.0, 0.0
			if cpu > 0 {
				cpuSet = 1
				samples = append(samples, prometheus.MustNewConstMetric(c.cpu, prometheus.GaugeValue, cpu, project, service, name))
			}
			if cfg.Memory > 0 {
				memorySet = 1
				samples = append(samples, prometheus.MustNewConstMetric(c.memory, prometheus.GaugeValue, float64(cfg.Memory), project, service, name))
			}
			samples = append(samples,
				prometheus.MustNewConstMetric(c.configured, prometheus.GaugeValue, cpuSet, project, service, name, "cpu"),
				prometheus.MustNewConstMetric(c.configured, prometheus.GaugeValue, memorySet, project, service, name, "memory"))
		}
	}
	for _, sample := range samples {
		ch <- sample
	}
	ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 1)
}
