package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jur4ikoff/model-service/internal/auth"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/utils/authjwt"
)

type authVerifierStub struct {
	principal auth.Principal
	err       error
}

func (s authVerifierStub) Verify(context.Context, string) (auth.Principal, error) {
	return s.principal, s.err
}

// Техника: классы эквивалентности проектных разрешений.
func TestModelRoutesRequireAuthorizedProjectGrant(t *testing.T) {
	model := &modelStub{}
	router := NewHandler(model, authVerifierStub{principal: auth.Principal{
		Subject:  "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1",
		Username: "alice",
		Project:  &authjwt.ProjectGrant{Name: "project-1", Permissions: authjwt.Permissions{CanReadModel: true, CanCreateModel: true}},
	}}).Router()

	apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-1/models", "", http.StatusUnauthorized)

	request := "Bearer access-token"
	response := apiRequestWithAuth(t, router, http.MethodGet, "/api/v1/projects/project-2/models", "", request, http.StatusForbidden)
	if response.Body.Len() == 0 {
		t.Fatal("authorization failure must return a problem response")
	}
	apiRequestWithAuth(t, router, http.MethodGet, "/api/v1/projects/project-1/models", "", request, http.StatusOK)
	apiRequestWithAuth(t, router, http.MethodPost, "/api/v1/projects/project-1/models", `{"name":"demo","spec":{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"org/model"}}}`, request, http.StatusCreated)
	if model.created.Username != "alice" {
		t.Fatalf("model creator came from request instead of JWT: %q", model.created.Username)
	}
}

// Техника: класс недоступной зависимости проверки JWT.
func TestUnavailableJWTVerifierReturnsServiceUnavailable(t *testing.T) {
	router := NewHandler(&modelStub{}, authVerifierStub{err: auth.ErrUnavailable}).Router()
	apiRequestWithAuth(t, router, http.MethodGet, "/api/v1/projects/project-1/models", "", "Bearer access-token", http.StatusServiceUnavailable)
}

// Техника: класс невалидного токена.
func TestInvalidJWTReturnsUnauthorized(t *testing.T) {
	router := NewHandler(&modelStub{}, authVerifierStub{err: errors.New("bad signature")}).Router()
	apiRequestWithAuth(t, router, http.MethodGet, "/api/v1/projects/project-1/models", "", "Bearer access-token", http.StatusUnauthorized)
}

func apiRequestWithAuth(t *testing.T, router http.Handler, method, path, body, authorization string, status int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", authorization)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s: got %d, want %d; body=%s", method, path, response.Code, status, response.Body.String())
	}
	return response
}

// Техника: классы эквивалентности проектных разрешений.
func TestDeploymentsHistoryRequiresProjectReadPermission(t *testing.T) {
	fake := &modelStub{deployment: domain.Deployment{ID: "deployment-1", ModelID: "model-1", Project: "demo", Name: "example", Username: "alice", Status: "stopped"}}
	router := NewHandler(fake, authVerifierStub{principal: auth.Principal{Project: &authjwt.ProjectGrant{Name: "demo", Permissions: authjwt.Permissions{CanReadModel: true}}}}).Router()
	path := "/api/v1/projects/demo/deployments/history"
	apiRequest(t, router, http.MethodGet, path, "", http.StatusUnauthorized)
	apiRequestWithAuth(t, router, http.MethodGet, "/api/v1/projects/other/deployments/history", "", "Bearer token", http.StatusForbidden)
	response := apiRequestWithAuth(t, router, http.MethodGet, path, "", "Bearer token", http.StatusOK)
	var body struct {
		Deployments []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Status   string `json:"status"`
		} `json:"deployments"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Deployments) != 1 || body.Deployments[0].ID != "deployment-1" || body.Deployments[0].Username != "alice" || body.Deployments[0].Status != "stopped" || fake.project != "demo" {
		t.Fatalf("unexpected history response: %s; err=%v", response.Body.String(), err)
	}
}

// Техника: классы эквивалентности проектных разрешений.
func TestActiveDeploymentsRequiresProjectReadPermission(t *testing.T) {
	fake := &modelStub{deployment: domain.Deployment{ID: "deployment-1", Project: "demo", Username: "alice", Status: "running"}}
	router := NewHandler(fake, authVerifierStub{principal: auth.Principal{Project: &authjwt.ProjectGrant{Name: "demo", Permissions: authjwt.Permissions{CanReadModel: true}}}}).Router()
	path := "/api/v1/projects/demo/deployments"
	apiRequest(t, router, http.MethodGet, path, "", http.StatusUnauthorized)
	apiRequestWithAuth(t, router, http.MethodGet, "/api/v1/projects/other/deployments", "", "Bearer token", http.StatusForbidden)
	response := apiRequestWithAuth(t, router, http.MethodGet, path, "", "Bearer token", http.StatusOK)
	var body struct {
		Deployments []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Status   string `json:"status"`
		} `json:"deployments"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Deployments) != 1 || body.Deployments[0].ID != "deployment-1" || body.Deployments[0].Username != "alice" || body.Deployments[0].Status != "running" || fake.project != "demo" {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

// Техника: класс записей с временными метками.
func TestDeploymentListsIncludeTimestamps(t *testing.T) {
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := started.Add(time.Minute)
	fake := &modelStub{deployment: domain.Deployment{ID: "deployment-1", Project: "demo", Status: "running", StartedAt: &started, UpdatedAt: &updated}}
	router := NewHandler(fake).Router()
	for _, path := range []string{"/api/v1/projects/demo/deployments", "/api/v1/projects/demo/deployments/history"} {
		response := apiRequest(t, router, http.MethodGet, path, "", http.StatusOK)
		var body struct {
			Deployments []struct {
				StartedAt *time.Time `json:"started_at"`
				UpdatedAt *time.Time `json:"updated_at"`
			} `json:"deployments"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Deployments) != 1 || body.Deployments[0].StartedAt == nil || body.Deployments[0].UpdatedAt == nil || !body.Deployments[0].StartedAt.Equal(started) || !body.Deployments[0].UpdatedAt.Equal(updated) {
			t.Fatalf("%s: %s", path, response.Body.String())
		}
	}
}
