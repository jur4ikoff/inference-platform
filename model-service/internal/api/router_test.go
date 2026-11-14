package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jur4ikoff/model-service/internal/auth"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/service"
)

// Техника: класс доступной документации.
func TestDocumentation(t *testing.T) {
	router := NewHandler(nil).Router()
	for _, test := range []struct {
		path, contentType, content string
	}{
		{"/api/v1/docs", "text/html", "SwaggerUIBundle"},
		{"/api/v1/openapi.yaml", "application/openapi+yaml", "openapi: 3.1.0"},
		{"/api/v1/openapi.json", "application/openapi+json", "Model Service API"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := apiRequest(t, router, http.MethodGet, test.path, "", http.StatusOK)
			if !strings.Contains(response.Header().Get("Content-Type"), test.contentType) {
				t.Errorf("unexpected content type: %s", response.Header().Get("Content-Type"))
			}
			if !strings.Contains(response.Body.String(), test.content) {
				t.Errorf("documentation does not contain %q", test.content)
			}
		})
	}

	response := apiRequest(t, router, http.MethodGet, "/api/v1/openapi.json", "", http.StatusOK)
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string                     `json:"operationId"`
			Responses   map[string]json.RawMessage `json:"responses"`
			RequestBody json.RawMessage            `json:"requestBody"`
			Parameters  []struct {
				Name     string `json:"name"`
				In       string `json:"in"`
				Required bool   `json:"required"`
			} `json:"parameters"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	for name, schema := range spec.Components.Schemas {
		if _, exists := schema.Properties["$schema"]; exists {
			t.Errorf("schema %q contains automatic $schema field", name)
		}
	}
	for path := range spec.Paths {
		if !strings.HasPrefix(path, "/api/v1/") {
			t.Errorf("unversioned OpenAPI path: %s", path)
		}
	}
	for _, test := range []struct {
		path, method, status string
		body                 bool
	}{
		{"/api/v1/projects/{project}/models", "post", "201", true},
		{"/api/v1/projects/{project}/models", "get", "200", false},
		{"/api/v1/projects/{project}/models/{model_id}", "get", "200", false},
		{"/api/v1/projects/{project}/models/{model_id}", "patch", "200", true},
		{"/api/v1/projects/{project}/models/{model_id}", "delete", "204", false},
		{"/api/v1/projects/{project}/models/{model_id}/deployment", "post", "202", false},
		{"/api/v1/projects/{project}/models/{model_id}/deployment", "get", "200", false},
		{"/api/v1/projects/{project}/models/{model_id}/deployment", "delete", "202", false},
	} {
		op := spec.Paths[test.path][test.method]
		if op.OperationID == "" || op.Responses[test.status] == nil || (test.body && len(op.RequestBody) == 0) {
			t.Errorf("incomplete documentation for %s %s: %+v", test.method, test.path, op)
		}
		if strings.Contains(test.path, "{model_id}") {
			if len(op.Parameters) < 2 || op.Parameters[0].Name != "project" || op.Parameters[0].In != "path" || op.Parameters[1].Name != "model_id" || op.Parameters[1].In != "path" {
				t.Errorf("missing project and model ID parameters for %s %s: %+v", test.method, test.path, op.Parameters)
			}
		}
	}
	parameters := spec.Paths["/api/v1/projects/{project}/models"]["get"].Parameters
	found := map[string]bool{}
	for _, parameter := range parameters {
		if parameter.In == "query" {
			found[parameter.Name] = true
		}
	}
	for _, name := range []string{"offset", "limit"} {
		if !found[name] {
			t.Errorf("missing list parameter %q in OpenAPI", name)
		}
	}
	if found["pagetoken"] || found["nextpagetoken"] {
		t.Fatal("nextpagetoken must only appear in responses")
	}
	if found["project"] {
		t.Fatal("project must not be a query parameter")
	}
	for _, method := range []string{"get", "post"} {
		foundProject := false
		for _, parameter := range spec.Paths["/api/v1/projects/{project}/models"][method].Parameters {
			if parameter.Name == "project" && parameter.In == "path" && parameter.Required {
				foundProject = true
			}
		}
		if !foundProject {
			t.Errorf("%s models must document a required project path parameter", method)
		}
	}
	if _, exists := spec.Components.Schemas["CreateModelRequest"].Properties["project"]; exists {
		t.Fatal("create body must not contain project")
	}
	if _, exists := spec.Paths["/api/v1/models"]; exists {
		t.Fatal("old model collection path is still documented")
	}
	if _, exists := spec.Paths["/api/v1/projects/{project}/models/{model_id}/interactions"]; exists {
		t.Fatal("removed model interaction path is still documented")
	}
	if _, exists := spec.Paths["/api/v1/projects/{project}/deployments"]["get"]; !exists {
		t.Fatal("active deployment list is not documented")
	}
}

// Техника: класс неверсионированных маршрутов.
func TestUnversionedRoutesNotFound(t *testing.T) {
	router := NewHandler(nil).Router()
	for _, path := range []string{"/models", "/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", "/docs", "/openapi.yaml", "/api/v2/models"} {
		apiRequest(t, router, http.MethodGet, path, "", http.StatusNotFound)
	}
}

// Техника: класс удалённых маршрутов.
func TestRemovedModelRoutesNotFound(t *testing.T) {
	router := NewHandler(nil).Router()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		apiRequest(t, router, method, "/api/v1/models?project=project-1", "", http.StatusNotFound)
	}
	apiRequest(t, router, http.MethodPost, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1/interactions", `{"input":{}}`, http.StatusNotFound)
}

// Техника: классы эквивалентности имён и идентификаторов.
func TestModelRoutesRejectNamesAsIdentifiers(t *testing.T) {
	router := NewHandler(nil).Router()
	for _, test := range []struct {
		method, suffix, body string
	}{
		{http.MethodGet, "", ""},
		{http.MethodPatch, "", `{"name":"renamed"}`},
		{http.MethodDelete, "", ""},
		{http.MethodPost, "/deployment", ""},
		{http.MethodGet, "/deployment", ""},
		{http.MethodDelete, "/deployment", ""},
	} {
		apiRequest(t, router, test.method, "/api/v1/projects/project-1/models/demo"+test.suffix, test.body, http.StatusUnprocessableEntity)
	}
	apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-1/models/by-id/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", "", http.StatusNotFound)
}

func TestCreateDeploymentRequiresIdempotencyKey(t *testing.T) {
	router := NewHandler(nil).Router()
	apiRequest(t, router, http.MethodPost, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1/deployment", "", http.StatusUnprocessableEntity)
}

// Техника: переходы состояний модели и deployment.
func TestModelRoutes(t *testing.T) {
	model := domain.Model{
		ID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", Name: "demo", Description: "description", Username: "user-1", Project: "project-1",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Spec: domain.ModelSpec{ProviderType: domain.ProviderHuggingFace, ProviderHuggingFace: &domain.HuggingFaceProvider{Repo: "org/model"}},
	}
	fake := &modelStub{model: model}
	router := NewHandler(fake).Router()
	response := apiRequest(t, router, http.MethodPost, "/api/v1/projects/project-1/models", `{"name":"demo","spec":{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"org/model"}}}`, http.StatusCreated)
	assertModelResponse(t, response, model)
	if fake.created.Name != "demo" || fake.created.Username != "" || fake.created.Project != "project-1" || fake.created.Spec.ProviderType != domain.ProviderHuggingFace || fake.created.Spec.ProviderHuggingFace == nil || fake.created.Spec.ProviderHuggingFace.Repo != "org/model" {
		t.Fatalf("unexpected create input: %+v", fake.created)
	}

	response = apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", "", http.StatusOK)
	assertModelResponse(t, response, model)
	if fake.project != model.Project || fake.modelID != model.ID {
		t.Fatalf("get used project %q and model ID %q", fake.project, fake.modelID)
	}

	response = apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-1/models", "", http.StatusOK)
	var list listModelsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil || len(list.Models) != 1 || list.Models[0].ID != model.ID || fake.project != "project-1" {
		t.Fatalf("unexpected filtered list: %s; project=%q; error=%v", response.Body.String(), fake.project, err)
	}

	response = apiRequest(t, router, http.MethodPatch, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", `{"description":""}`, http.StatusOK)
	assertModelResponse(t, response, model)
	if fake.project != model.Project || fake.modelID != model.ID || fake.updated.Name != nil || fake.updated.Spec != nil || fake.updated.Description == nil || *fake.updated.Description != "" {
		t.Fatalf("partial update did not preserve omitted fields: %+v", fake.updated)
	}

	response = apiRequest(t, router, http.MethodDelete, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", "", http.StatusNoContent)
	if response.Body.Len() != 0 || fake.project != model.Project || fake.modelID != model.ID {
		t.Fatalf("unexpected delete response: %s; project=%q; model_id=%q", response.Body.String(), fake.project, fake.modelID)
	}

	fake.deployment = domain.Deployment{ID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f2", ModelID: model.ID, Project: model.Project, Name: model.Name, Status: "starting"}
	fake.deploymentAccepted = true
	createRouter := NewHandler(fake).WithIdempotencyStore(passthroughIdempotency{}).Router()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1/deployment", nil)
	request.Header.Set("Idempotency-Key", "test-key")
	request = request.WithContext(context.WithValue(request.Context(), principalContextKey{}, auth.Principal{Subject: "user-1"}))
	response = httptest.NewRecorder()
	createRouter.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create deployment: status=%d body=%s", response.Code, response.Body)
	}
	if response.Header().Get("Location") != "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1/deployment" {
		t.Fatalf("unexpected deployment location: %q", response.Header().Get("Location"))
	}
	assertDeploymentResponse(t, response, fake.deployment)

	response = apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1/deployment", "", http.StatusOK)
	assertDeploymentResponse(t, response, fake.deployment)

	response = apiRequest(t, router, http.MethodDelete, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1/deployment", "", http.StatusAccepted)
	if response.Body.Len() != 0 {
		t.Fatalf("stop response must be empty: %s", response.Body.String())
	}
	fake.deploymentAccepted = false
	apiRequest(t, router, http.MethodDelete, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1/deployment", "", http.StatusNoContent)

	fake.model = domain.Model{}
	response = apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-1/models", "", http.StatusOK)
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil || list.Models == nil || len(list.Models) != 0 || list.NextPageToken != nil || fake.project != "project-1" {
		t.Fatalf("expected empty list: %s; project=%q", response.Body.String(), fake.project)
	}
}

// Техника: классы эквивалентности и граничные значения HTTP-запросов.
func TestRequestValidation(t *testing.T) {
	router := NewHandler(nil).Router()
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"malformed JSON", `{"name":`, http.StatusBadRequest},
		{"missing required fields", `{}`, http.StatusUnprocessableEntity},
		{"unknown field", `{"name":"demo","username":"u","spec":{},"unknown":true}`, http.StatusUnprocessableEntity},
		{"unknown specification field", `{"name":"demo","username":"u","spec":{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"org/model","unknown":true}}}`, http.StatusUnprocessableEntity},
		{"unsupported provider", `{"name":"demo","spec":{"provider_type":"OTHER","provider_hugging_face":{"repo":"org/model"}}}`, http.StatusUnprocessableEntity},
		{"project ID in body", `{"name":"demo","username":"u","project":"other-project","spec":{}}`, http.StatusUnprocessableEntity},
		{"body limit", `{"name":"` + strings.Repeat("a", (1<<20)+1) + `"}`, http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			apiRequest(t, router, http.MethodPost, "/api/v1/projects/project-1/models", test.body, test.status)
		})
	}
}

// Техника: классы эквивалентности ошибок сервиса.
func TestServiceErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{fmt.Errorf("create: %w", service.ErrInvalidInput), http.StatusBadRequest},
		{fmt.Errorf("get: %w", domain.ErrModelNotFound), http.StatusNotFound},
		{domain.ErrModelAlreadyExists, http.StatusConflict},
		{service.ErrRuntimeUnavailable, http.StatusServiceUnavailable},
		{errors.New("private database details"), http.StatusInternalServerError},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			router := NewHandler(&modelStub{err: test.err}).Router()
			response := apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", "", test.status)
			var problem struct {
				Status int `json:"status"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil || problem.Status != test.status || strings.Contains(response.Body.String(), "private database details") {
				t.Fatalf("unexpected error response: %s; error=%v", response.Body.String(), err)
			}
		})
	}
}

func apiRequest(t *testing.T, router http.Handler, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s: got %d, want %d; body=%s", method, path, response.Code, status, response.Body.String())
	}
	return response
}

func assertModelResponse(t *testing.T, response *httptest.ResponseRecorder, expected domain.Model) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["$schema"]; exists {
		t.Fatal("model response contains automatic $schema field")
	}
	var model modelResponseBody
	if err := json.Unmarshal(response.Body.Bytes(), &model); err != nil {
		t.Fatal(err)
	}
	if model.ID != expected.ID || model.Name != expected.Name || model.Username != expected.Username || model.Project != expected.Project || model.Description != expected.Description || !model.CreatedAt.Equal(expected.CreatedAt) || !model.UpdatedAt.Equal(expected.UpdatedAt) || model.Spec.ProviderType != string(expected.Spec.ProviderType) || model.Spec.ProviderHuggingFace == nil || expected.Spec.ProviderHuggingFace == nil || model.Spec.ProviderHuggingFace.Repo != expected.Spec.ProviderHuggingFace.Repo {
		t.Fatalf("unexpected model response: %s", response.Body.String())
	}
}

func assertDeploymentResponse(t *testing.T, response *httptest.ResponseRecorder, expected domain.Deployment) {
	t.Helper()
	var deployment deploymentResponse
	if err := json.Unmarshal(response.Body.Bytes(), &deployment); err != nil {
		t.Fatal(err)
	}
	if deployment.ID != expected.ID || deployment.ModelID != expected.ModelID || deployment.Project != expected.Project || deployment.Status != expected.Status {
		t.Fatalf("unexpected deployment response: %s", response.Body.String())
	}
}

type modelStub struct {
	model              domain.Model
	err                error
	project            string
	modelID            string
	created            service.CreateModelInput
	updated            service.UpdateModelInput
	deployment         domain.Deployment
	deploymentAccepted bool
}

type passthroughIdempotency struct{}

func (passthroughIdempotency) Execute(_ context.Context, _, _, _, _ string, run func() ([]byte, int, error)) ([]byte, error) {
	data, _, err := run()
	return data, err
}

// Техника: классы эквивалентности идентификаторов модели.
func TestGetModelUsesID(t *testing.T) {
	model := domain.Model{ID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", Project: "project-1", Name: "renamed", Spec: domain.ModelSpec{ProviderType: domain.ProviderHuggingFace, ProviderHuggingFace: &domain.HuggingFaceProvider{Repo: "org/model"}}}
	fake := &modelStub{model: model}
	router := NewHandler(fake).Router()
	response := apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", "", http.StatusOK)
	assertModelResponse(t, response, model)
	if fake.project != model.Project || fake.modelID != model.ID {
		t.Fatalf("incorrect selector: project=%q model_id=%q", fake.project, fake.modelID)
	}
	fake.err = domain.ErrModelNotFound
	apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-2/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", "", http.StatusNotFound)
}

func (s *modelStub) Create(_ context.Context, input service.CreateModelInput) (domain.Model, error) {
	s.created = input
	return s.model, s.err
}

func (s *modelStub) Get(_ context.Context, project, modelID string) (domain.Model, error) {
	s.project, s.modelID = project, modelID
	return s.model, s.err
}

func (s *modelStub) List(_ context.Context, input service.ListModelsInput) (service.ModelsPage, error) {
	s.project = input.Project
	if s.model.ID == "" {
		return service.ModelsPage{}, s.err
	}
	return service.ModelsPage{Models: []domain.Model{s.model}, Statuses: map[string]string{s.model.ID: "running"}}, s.err
}

func (s *modelStub) Update(_ context.Context, project, modelID string, input service.UpdateModelInput) (domain.Model, error) {
	s.project, s.modelID, s.updated = project, modelID, input
	return s.model, s.err
}

func (s *modelStub) Delete(_ context.Context, project, modelID string) error {
	s.project, s.modelID = project, modelID
	return s.err
}

func (s *modelStub) CreateDeployment(_ context.Context, project, modelID string) (domain.Deployment, error) {
	s.project, s.modelID = project, modelID
	return s.deployment, s.err
}

func (s *modelStub) GetDeployment(_ context.Context, project, modelID string) (domain.Deployment, error) {
	s.project, s.modelID = project, modelID
	return s.deployment, s.err
}

func (s *modelStub) GetDeploymentsHistory(_ context.Context, project string) ([]domain.Deployment, error) {
	s.project = project
	if s.deployment.ID == "" {
		return []domain.Deployment{}, s.err
	}
	return []domain.Deployment{s.deployment}, s.err
}

func (s *modelStub) StopDeployment(_ context.Context, project, modelID string) (bool, error) {
	s.project, s.modelID = project, modelID
	return s.deploymentAccepted, s.err
}

func (s *modelStub) GetActiveDeployments(ctx context.Context, project string) ([]domain.Deployment, error) {
	return s.GetDeploymentsHistory(ctx, project)
}

// Техника: класс модели со статусом.
func TestListModelsIncludesStatus(t *testing.T) {
	router := NewHandler(&modelStub{model: domain.Model{ID: "model-1"}}).Router()
	response := apiRequest(t, router, http.MethodGet, "/api/v1/projects/demo/models", "", http.StatusOK)
	var body struct {
		Models []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"models"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Models) != 1 || body.Models[0].ID != "model-1" || body.Models[0].Status != "running" {
		t.Fatalf("unexpected list response: %s", response.Body.String())
	}
}

// Техника: класс активного deployment.
func TestDeleteModelWithActiveDeploymentReturnsConflict(t *testing.T) {
	router := NewHandler(&modelStub{err: service.ErrActiveDeployments}).Router()
	apiRequest(t, router, http.MethodDelete, "/api/v1/projects/ml/models/e75957e1-6ac2-403c-8154-103d8d9c333d", "", http.StatusConflict)
}
