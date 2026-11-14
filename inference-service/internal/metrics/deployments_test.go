package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jur4ikoff/inference-service/internal/testutil"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/prometheus/client_golang/prometheus"
)

type testDB struct{ fail bool }

func (db testDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	if db.fail {
		return nil, errors.New("database unavailable")
	}
	return &testutil.Rows{Data: [][]any{
		{"one", "model-a", "model-1", "container-1", "model-a-x", "alice", "ml", "RUNNING"},
		{"two", "model-b", "model-2", "container-2", "model-b-x", "alice", "ml", "STOPPING"},
		{"three", "model-c", "model-3", "container-3", "model-c-x", "bob", "ml", "RUNNING"},
	}}, nil
}

type testDocker struct{ fail bool }

func (d testDocker) GetContainer(_ context.Context, id string) (client.ContainerInspectResult, error) {
	if d.fail {
		return client.ContainerInspectResult{}, errors.New("docker unavailable")
	}
	return client.ContainerInspectResult{Container: container.InspectResponse{State: &container.State{Running: id != "container-2"}}}, nil
}

func collectDeploymentMetrics(t *testing.T, db testDB, docker testDocker) (float64, map[string]float64) {
	t.Helper()
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewDeploymentsCollector(db, docker))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]float64{}
	success := -1.0
	for _, family := range families {
		switch family.GetName() {
		case "inference_deployment_metrics_scrape_success":
			success = family.Metric[0].Gauge.GetValue()
		case "inference_user_running_containers":
			for _, metric := range family.Metric {
				for _, label := range metric.Label {
					if label.GetName() == "username" {
						counts[label.GetValue()] = metric.Gauge.GetValue()
					}
				}
			}
		}
	}
	return success, counts
}

// Техника: разбиение на классы эквивалентности; случай доступных БД и Docker с несколькими владельцами контейнеров.
func TestDeploymentMetricsCountsRunningContainers(t *testing.T) {
	success, counts := collectDeploymentMetrics(t, testDB{}, testDocker{})
	if success != 1 || counts["alice"] != 1 || counts["bob"] != 1 {
		t.Fatalf("success=%v counts=%v", success, counts)
	}
}

// Техника: разбиение на классы эквивалентности; случай ошибки запроса к БД.
func TestDeploymentMetricsReportsDatabaseFailure(t *testing.T) {
	success, counts := collectDeploymentMetrics(t, testDB{fail: true}, testDocker{})
	if success != 0 || len(counts) != 0 {
		t.Fatalf("success=%v partial counts=%v", success, counts)
	}
}

// Техника: разбиение на классы эквивалентности; случай ошибки проверки контейнера в Docker.
func TestDeploymentMetricsReportsDockerFailure(t *testing.T) {
	success, counts := collectDeploymentMetrics(t, testDB{}, testDocker{fail: true})
	if success != 0 || len(counts) != 0 {
		t.Fatalf("success=%v partial counts=%v", success, counts)
	}
}
