package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReportUnavailableService(t *testing.T) {
	old := probes
	defer func() { probes = old }()
	probes = []probe{{name: "invalid", network: "http", address: ":invalid"}}
	response := httptest.NewRecorder()
	report(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var body struct {
		Status   string            `json:"status"`
		Services map[string]string `json:"services"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusServiceUnavailable || body.Status != "degraded" || body.Services["invalid"] != "down" {
		t.Fatalf("status=%d body=%s err=%v", response.Code, response.Body.String(), err)
	}
}

func TestReadinessReportsFailedCheck(t *testing.T) {
	old := readinessProbes
	defer func() { readinessProbes = old }()
	readinessProbes = []probe{{name: "model-service", network: "http", address: ":invalid"}}
	response := httptest.NewRecorder()
	readiness(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusServiceUnavailable || body.Status != "not_ready" || body.Checks["model-service"] != "not_ready" {
		t.Fatalf("status=%d body=%s err=%v", response.Code, response.Body.String(), err)
	}
}

func TestReadinessIncludesInference(t *testing.T) {
	for _, p := range readinessProbes {
		if p.name == "inference-service" && p.address == "http://inference-service:8080/healthz" {
			return
		}
	}
	t.Fatal("inference-service availability check is missing")
}
