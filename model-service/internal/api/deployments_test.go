package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestDeploymentEndpointsAreStubs(t *testing.T) {
	// A nil service ensures these contracts cannot invoke model lifecycle operations.
	router := NewHandler(nil).Router()
	const deployment = "/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1/deployment"
	for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
		apiRequest(t, router, method, deployment, "", http.StatusNotImplemented)
	}
	apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-1/deployments?offset=10&limit=5", "", http.StatusNotImplemented)
	for _, query := range []string{"?offset=-1", "?limit=0", "?limit=101"} {
		apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-1/deployments"+query, "", http.StatusUnprocessableEntity)
	}
	for _, action := range []string{"start", "stop"} {
		apiRequest(t, router, http.MethodPost,
			"/api/v1/projects/project-1/models/0197eb5a-37c6-7e48-8cd6-0c19edcd72f1/"+action, "", http.StatusNotFound)
	}
}

func TestDeploymentOpenAPIContracts(t *testing.T) {
	response := apiRequest(t, NewHandler(nil).Router(), http.MethodGet, "/api/v1/openapi.json", "", http.StatusOK)
	var document struct {
		Paths map[string]map[string]struct {
			Description string          `json:"description"`
			RequestBody json.RawMessage `json:"requestBody"`
			Responses   map[string]struct {
				Headers map[string]json.RawMessage `json:"headers"`
			} `json:"responses"`
			Parameters []struct {
				Name string `json:"name"`
				In   string `json:"in"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	const path = "/api/v1/projects/{project}/models/{model_id}/deployment"
	for _, method := range []string{"post", "get", "delete"} {
		op := document.Paths[path][method]
		if op.Description == "" || len(op.RequestBody) != 0 {
			t.Errorf("%s deployment must have a description and no request body", method)
		}
		if _, ok := op.Responses["501"]; !ok {
			t.Errorf("%s deployment must document 501", method)
		}
	}
	if _, ok := document.Paths[path]["post"].Responses["201"].Headers["Location"]; !ok {
		t.Fatal("deployment creation must document Location on 201")
	}
	params := document.Paths["/api/v1/projects/{project}/deployments"]["get"].Parameters
	queries := map[string]bool{}
	for _, param := range params {
		if param.In == "query" {
			queries[param.Name] = true
		}
	}
	if len(queries) != 2 || !queries["offset"] || !queries["limit"] {
		t.Fatalf("deployment list must accept offset and limit: %v", queries)
	}
}
