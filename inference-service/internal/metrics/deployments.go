package metrics

import (
	"context"
	"time"

	"github.com/containerd/errdefs"
	"github.com/jackc/pgx/v5"
	"github.com/moby/moby/client"
	"github.com/prometheus/client_golang/prometheus"
)

// DeploymentsCollector reads current deployments and checks their Docker state.
type DeploymentsCollector struct {
	pool                          deploymentDB
	runtime                       dockerInspector
	info, running, count, success *prometheus.Desc
}

type deploymentDB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}
type dockerInspector interface {
	GetContainer(context.Context, string) (client.ContainerInspectResult, error)
}

func NewDeploymentsCollector(pool deploymentDB, runtime dockerInspector) *DeploymentsCollector {
	return &DeploymentsCollector{pool: pool, runtime: runtime,
		info:    prometheus.NewDesc("inference_deployment_info", "Current nonterminal deployment information and database status.", []string{"deployment_id", "deployment", "model_id", "container_id", "container_name", "username", "project", "status"}, nil),
		running: prometheus.NewDesc("inference_deployment_container_running", "Whether Docker reports the deployment container as running.", []string{"deployment_id", "deployment", "model_id", "container_id", "container_name", "username", "project"}, nil),
		count:   prometheus.NewDesc("inference_user_running_containers", "Number of running model containers by initiating user and project.", []string{"username", "project"}, nil),
		success: prometheus.NewDesc("inference_deployment_metrics_scrape_success", "Whether deployment and Docker state were collected successfully.", nil, nil),
	}
}

func (c *DeploymentsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.info
	ch <- c.running
	ch <- c.count
	ch <- c.success
}

func (c *DeploymentsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	rows, err := c.pool.Query(ctx, `SELECT id::text, deployment_name, model_id::text, container_id, name, username, project, status FROM deployments WHERE status NOT IN ('STOPPED', 'FAILED')`)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 0)
		return
	}
	type deployment struct{ id, name, model, container, containerName, username, project, status string }
	var deployments []deployment
	for rows.Next() {
		var d deployment
		if err := rows.Scan(&d.id, &d.name, &d.model, &d.container, &d.containerName, &d.username, &d.project, &d.status); err != nil {
			rows.Close()
			ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 0)
			return
		}
		deployments = append(deployments, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 0)
		return
	}
	type group struct{ username, project string }
	counts := make(map[group]float64)
	var samples []prometheus.Metric
	for _, d := range deployments {
		key := group{d.username, d.project}
		if _, ok := counts[key]; !ok {
			counts[key] = 0
		}
		info, err := c.runtime.GetContainer(ctx, d.container)
		if err != nil && !errdefs.IsNotFound(err) {
			ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 0)
			return
		}
		running := 0.0
		if err == nil && info.Container.State != nil && info.Container.State.Running {
			running = 1
			counts[key]++
		}
		labels := []string{d.id, d.name, d.model, d.container, d.containerName, d.username, d.project}
		samples = append(samples, prometheus.MustNewConstMetric(c.running, prometheus.GaugeValue, running, labels...), prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1, append(labels, d.status)...))
	}
	for key, count := range counts {
		samples = append(samples, prometheus.MustNewConstMetric(c.count, prometheus.GaugeValue, count, key.username, key.project))
	}
	for _, sample := range samples {
		ch <- sample
	}
	ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 1)
}
