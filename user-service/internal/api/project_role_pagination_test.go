package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jur4ikoff/user-service/internal/api/handler"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

func TestListRolesPagination(t *testing.T) {
	repo := &rolesListRepository{roles: []domain.Role{
		{ID: 1, Name: "reader", CanReadProject: true},
		{ID: 2, Name: "runner", CanStartModel: true},
		{ID: 3, Name: "writer", CanUpdateModel: true},
	}}
	auth := &fakeAuth{principal: service.Principal{UserID: testUserID}}
	router := NewRouter(handler.NewAuthHandler(auth), handler.NewUserHandler(auth), handler.NewProjectHandler(nil), handler.NewRoleHandler(service.NewRoleService(repo)), "test")
	for _, test := range []struct {
		query string
		count int
		first int
		next  int
	}{
		{"?offset=0&limit=2", 2, 1, 2},
		{"?offset=2&limit=2", 1, 3, 0},
		{"?offset=20&limit=2", 0, 0, 0},
		{"?name=runner&limit=2", 1, 2, 0},
		{"", 3, 1, 0},
	} {
		response := listRequest(t, router, http.MethodGet, "/api/v1/roles"+test.query, "", true, http.StatusOK)
		var page handler.ListRolesResponse
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Roles == nil || len(page.Roles) != test.count || (test.count > 0 && page.Roles[0].ID != test.first) {
			t.Fatalf("query=%s response=%s", test.query, response.Body.String())
		}
		checkNextOffset(t, page.NextPageToken, test.next)
		if test.first == 2 && !page.Roles[0].CanStartModel {
			t.Fatal("role permissions were lost in the response")
		}
	}
	if repo.limit != 21 {
		t.Fatalf("default page should fetch 21 rows, got %d", repo.limit)
	}
	for _, query := range []string{"?offset=-1", "?limit=0", "?limit=101", "?limit=invalid"} {
		listRequest(t, router, http.MethodGet, "/api/v1/roles"+query, "", true, http.StatusUnprocessableEntity)
	}
	listRequest(t, router, http.MethodGet, "/api/v1/roles", "", false, http.StatusUnauthorized)
}

func TestProjectListAndIDPaths(t *testing.T) {
	repo := &projectsListRepository{projects: []domain.Project{
		{ID: "project-a", Name: "demo"}, {ID: "project-b", Name: "other"}, {ID: "project-c", Name: "demo"},
	}}
	auth := &fakeAuth{principal: service.Principal{UserID: testUserID}}
	router := NewRouter(handler.NewAuthHandler(auth), handler.NewUserHandler(auth), handler.NewProjectHandler(service.NewProjectService(repo)), handler.NewRoleHandler(nil), "test")
	for _, test := range []struct {
		query string
		first domain.ProjectID
		next  int
	}{
		{"?name=demo&offset=0&limit=1", "project-a", 1},
		{"?name=demo&offset=1&limit=1", "project-c", 0},
		{"?offset=10&limit=1", "", 0},
	} {
		response := listRequest(t, router, http.MethodGet, "/api/v1/projects"+test.query, "", true, http.StatusOK)
		var page handler.ListProjectsResponse
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Projects == nil || (test.first == "" && len(page.Projects) != 0) || (test.first != "" && (len(page.Projects) != 1 || page.Projects[0].ID != string(test.first))) {
			t.Fatalf("query=%s response=%s", test.query, response.Body.String())
		}
		checkNextOffset(t, page.NextPageToken, test.next)
		if repo.userID != testUserID {
			t.Fatal("project selection must use the authenticated user")
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		body := ""
		if method == http.MethodPatch {
			body = `{"name":"renamed"}`
		}
		listRequest(t, router, method, "/api/v1/projects/"+testUserID, body, true, http.StatusNotImplemented)
		listRequest(t, router, method, "/api/v1/projects/invalid-id", body, true, http.StatusUnprocessableEntity)
	}
	listRequest(t, router, http.MethodGet, "/api/v1/projects/list", "", true, http.StatusUnprocessableEntity)
	listRequest(t, router, http.MethodGet, "/api/v1/projects?limit=0", "", true, http.StatusUnprocessableEntity)
	listRequest(t, router, http.MethodGet, "/api/v1/projects", "", false, http.StatusUnauthorized)
}

func TestUserServicePaginationOpenAPI(t *testing.T) {
	auth := &fakeAuth{}
	router := NewRouter(handler.NewAuthHandler(auth), handler.NewUserHandler(auth), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
	response := listRequest(t, router, http.MethodGet, "/api/v1/openapi.json", "", false, http.StatusOK)
	var document struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name     string `json:"name"`
				In       string `json:"in"`
				Required bool   `json:"required"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/users", "/api/v1/projects", "/api/v1/roles"} {
		found := map[string]bool{}
		for _, parameter := range document.Paths[path]["get"].Parameters {
			found[parameter.Name] = true
		}
		if !found["offset"] || !found["limit"] || found["pagetoken"] || found["nextpagetoken"] {
			t.Errorf("wrong pagination parameters for %s: %v", path, found)
		}
	}
	for _, method := range []string{"get", "patch", "delete"} {
		params := document.Paths["/api/v1/projects/{id}"][method].Parameters
		if len(params) != 1 || params[0].Name != "id" || params[0].In != "path" || !params[0].Required {
			t.Errorf("project %s must use required ID path parameter", method)
		}
	}
	if _, exists := document.Paths["/api/v1/projects/list"]; exists {
		t.Fatal("old project list route is still documented")
	}
}

func listRequest(t *testing.T, router http.Handler, method, path, body string, authenticated bool, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if authenticated {
		request.Header.Set("Authorization", "Bearer token")
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s: status=%d, want=%d: %s", method, path, response.Code, wantStatus, response.Body.String())
	}
	return response
}

func checkNextOffset(t *testing.T, next *int, want int) {
	t.Helper()
	if (want == 0 && next != nil) || (want != 0 && (next == nil || *next != want)) {
		t.Fatalf("next offset=%v, want=%d (zero means null)", next, want)
	}
}

type rolesListRepository struct {
	roles []domain.Role
	limit int
}

func (r *rolesListRepository) List(_ context.Context, name string, offset, limit int) ([]domain.Role, error) {
	r.limit = limit
	var roles []domain.Role
	for _, role := range r.roles {
		if name == "" || role.Name == name {
			roles = append(roles, role)
		}
	}
	if offset >= len(roles) {
		return nil, nil
	}
	return roles[offset:min(offset+limit, len(roles))], nil
}

type projectsListRepository struct {
	service.ProjectRepository
	projects []domain.Project
	userID   domain.UserID
}

func (r *projectsListRepository) List(_ context.Context, userID domain.UserID, name string, offset, limit int) ([]domain.Project, error) {
	r.userID = userID
	var projects []domain.Project
	for _, project := range r.projects {
		if name == "" || project.Name == name {
			projects = append(projects, project)
		}
	}
	if offset >= len(projects) {
		return nil, nil
	}
	return projects[offset:min(offset+limit, len(projects))], nil
}
