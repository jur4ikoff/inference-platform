package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jur4ikoff/user-service/internal/api/handler"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

func TestRegistrationRoutes(t *testing.T) {
	const path = "/api/v1/auth/register"
	for _, tt := range []struct {
		name   string
		body   string
		err    error
		status int
		calls  int
	}{
		{"success without token", `{"firstname":"Иван","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, nil, http.StatusCreated, 1},
		{"duplicate email", `{"firstname":"Иван","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, domain.ErrEmailAlreadyTaken, http.StatusConflict, 1},
		{"invalid input", `{"firstname":"Иван","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, service.ErrInvalidInput, http.StatusBadRequest, 1},
		{"storage error", `{"firstname":"Иван","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, errors.New("private database error"), http.StatusInternalServerError, 1},
		{"invalid email", `{"firstname":"Иван","lastname":"Попов","email":"invalid","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"short password", `{"firstname":"Иван","lastname":"Попов","email":"user@example.com","password":"short"}`, nil, http.StatusUnprocessableEntity, 0},
		{"missing password", `{"firstname":"Иван","lastname":"Попов","email":"user@example.com"}`, nil, http.StatusUnprocessableEntity, 0},
		{"missing email", `{"firstname":"Иван","lastname":"Попов","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"missing firstname", `{"lastname":"Попов","email":"user@example.com","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"missing lastname", `{"firstname":"Иван","email":"user@example.com","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"empty firstname", `{"firstname":"","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"blank lastname", `{"firstname":"Иван","lastname":"   ","email":"user@example.com","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"role is not a user field", `{"firstname":"Иван","lastname":"Попов","email":"user@example.com","password":"password-12345","role":"admin"}`, nil, http.StatusUnprocessableEntity, 0},
	} {
		t.Run(path+"/"+tt.name, func(t *testing.T) {
			fake := &fakeAuth{registerErr: tt.err}
			router := NewRouter(handler.NewAuthHandler(fake), handler.NewUserHandler(fake), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
			request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(tt.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.status || fake.registerCalls != tt.calls {
				t.Fatalf("status = %d, calls = %d; want %d, %d; body: %s", response.Code, fake.registerCalls, tt.status, tt.calls, response.Body.String())
			}
			if tt.calls > 0 && fake.registerInput != (service.RegisterInput{Firstname: "Иван", Lastname: "Попов", Email: "user@example.com", Password: "password-12345"}) {
				t.Fatal("registration body was not passed to service")
			}
			if tt.status == http.StatusCreated {
				var user map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &user); err != nil {
					t.Fatal(err)
				}
				if len(user) != 4 || user["id"] != testUserID || user["email"] != "user@example.com" || user["firstname"] != "Иван" || user["lastname"] != "Попов" {
					t.Fatalf("unexpected registration response: %v", user)
				}
			}
			if strings.Contains(response.Body.String(), "private database error") || strings.Contains(response.Body.String(), "password-12345") {
				t.Fatal("response exposed password or internal error")
			}
		})
	}
}

func TestRegistrationOpenAPIAndRemovedCreateUser(t *testing.T) {
	fake := &fakeAuth{}
	router := NewRouter(handler.NewAuthHandler(fake), handler.NewUserHandler(fake), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/user", strings.NewReader(`{}`)))
	if response.Code != http.StatusNotFound || fake.registerCalls != 0 {
		t.Fatalf("removed createUser: status = %d, calls = %d", response.Code, fake.registerCalls)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil))
	var document struct {
		Paths map[string]struct {
			Post struct {
				OperationID string `json:"operationId"`
			} `json:"post"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					WriteOnly bool `json:"writeOnly"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Paths["/api/v1/auth/register"].Post.OperationID != "registerUser" || strings.Contains(response.Body.String(), "createUser") {
		t.Fatal("OpenAPI must expose registerUser as the only registration operation")
	}
	if _, exists := document.Paths["/api/v1/user"]; exists {
		t.Fatal("OpenAPI contains the removed createUser route")
	}
	schema := document.Components.Schemas["RegisterRequest"]
	if len(schema.Required) != 4 || !schema.Properties["password"].WriteOnly {
		t.Fatal("registration must document all four required fields and a write-only password")
	}
	for _, field := range []string{"firstname", "lastname", "email", "password"} {
		found := false
		for _, required := range schema.Required {
			found = found || required == field
		}
		if !found {
			t.Errorf("registration field %q is not required", field)
		}
	}
}

func TestListUsersRoute(t *testing.T) {
	users := []service.RegisteredUser{
		{ID: testUserID, Email: "user@example.com"},
		{ID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f2", Email: "admin@example.com"},
	}
	for _, tt := range []struct {
		name   string
		token  string
		users  []service.RegisteredUser
		err    error
		status int
		calls  int
	}{
		{"missing token", "", users, nil, http.StatusUnauthorized, 0},
		{"malformed token", "invalid", users, nil, http.StatusUnauthorized, 0},
		{"authenticated user", "Bearer token", users, nil, http.StatusOK, 1},
		{"empty list", "Bearer token", nil, nil, http.StatusOK, 1},
		{"storage error", "Bearer token", nil, errors.New("private database error"), http.StatusInternalServerError, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeAuth{principal: service.Principal{UserID: testUserID}, users: tt.users, listErr: tt.err}
			router := NewRouter(handler.NewAuthHandler(fake), handler.NewUserHandler(fake), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
			request := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
			request.Header.Set("Authorization", tt.token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.status || fake.listCalls != tt.calls {
				t.Fatalf("status = %d, calls = %d; want %d, %d; body: %s", response.Code, fake.listCalls, tt.status, tt.calls, response.Body.String())
			}
			if tt.status == http.StatusOK {
				var got handler.ListUsersResponse
				if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				want := make([]handler.RegisteredUserResponse, 0, len(tt.users))
				for _, user := range tt.users {
					want = append(want, handler.RegisteredUserResponse{ID: string(user.ID), Email: user.Email, Firstname: user.Firstname, Lastname: user.Lastname})
				}
				if !reflect.DeepEqual(got.Users, want) {
					t.Fatalf("users = %#v, want %#v", got.Users, want)
				}
			}
			if strings.Contains(response.Body.String(), "private database error") || strings.Contains(response.Body.String(), "password") {
				t.Fatal("response exposed password or internal error")
			}
		})
	}
}

func TestListUsersPaginationParameters(t *testing.T) {
	for _, test := range []struct {
		query  string
		status int
		want   service.ListUsersInput
	}{
		{"", http.StatusOK, service.ListUsersInput{Limit: 20}},
		{"?offset=10&limit=5", http.StatusOK, service.ListUsersInput{Offset: 10, Limit: 5}},
		{"?offset=-1", http.StatusUnprocessableEntity, service.ListUsersInput{}},
		{"?limit=0", http.StatusUnprocessableEntity, service.ListUsersInput{}},
		{"?limit=101", http.StatusUnprocessableEntity, service.ListUsersInput{}},
		{"?offset=invalid", http.StatusUnprocessableEntity, service.ListUsersInput{}},
	} {
		t.Run(test.query, func(t *testing.T) {
			nextOffset := 15
			fake := &fakeAuth{principal: service.Principal{UserID: testUserID}, nextPageToken: &nextOffset}
			router := NewRouter(handler.NewAuthHandler(fake), handler.NewUserHandler(fake), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
			request := httptest.NewRequest(http.MethodGet, "/api/v1/users"+test.query, nil)
			request.Header.Set("Authorization", "Bearer token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
			if test.status == http.StatusOK {
				var got handler.ListUsersResponse
				if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if fake.listInput != test.want || (got.NextPageToken == nil || *got.NextPageToken != nextOffset) || got.Users == nil {
					t.Fatalf("pagination input=%+v response=%+v", fake.listInput, got)
				}
			} else if fake.listCalls != 0 {
				t.Fatal("invalid pagination reached the service")
			}
		})
	}
}

func TestUpdateUserProfilePermissions(t *testing.T) {
	const otherID = "0197eb5a-37c6-7e48-8cd6-0c19edcd72f2"
	for _, test := range []struct {
		name, token, target, body string
		status, calls             int
	}{
		{"owner profile", "Bearer token", testUserID, `{"firstname":"Иван","lastname":"Попов"}`, http.StatusOK, 1},
		{"owner password", "Bearer token", testUserID, `{"password":"new-password-456"}`, http.StatusOK, 1},
		{"no token", "", testUserID, `{"firstname":"Иван"}`, http.StatusUnauthorized, 0},
		{"another user", "Bearer token", otherID, `{"firstname":"Иван"}`, http.StatusForbidden, 0},
		{"self promotion", "Bearer token", testUserID, `{"role":"admin","firstname":"Иван"}`, http.StatusUnprocessableEntity, 0},
		{"owner role", "Bearer token", testUserID, `{"role":"user"}`, http.StatusUnprocessableEntity, 0},
		{"invalid role", "Bearer token", otherID, `{"role":"editor"}`, http.StatusUnprocessableEntity, 0},
		{"creation time immutable", "Bearer token", testUserID, `{"created_at":"2026-10-22T12:00:00Z"}`, http.StatusUnprocessableEntity, 0},
		{"hash not accepted", "Bearer token", testUserID, `{"password_hash":"hash"}`, http.StatusUnprocessableEntity, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeAuth{principal: service.Principal{UserID: testUserID}}
			router := NewRouter(handler.NewAuthHandler(fake), handler.NewUserHandler(fake), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
			request := httptest.NewRequest(http.MethodPatch, "/api/v1/users/"+test.target, strings.NewReader(test.body))
			request.Header.Set("Authorization", test.token)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status || fake.updateCalls != test.calls {
				t.Fatalf("status = %d, calls = %d; want %d, %d; body: %s", response.Code, fake.updateCalls, test.status, test.calls, response.Body.String())
			}
			if test.calls > 0 {
				var want service.UpdateUserInput
				if err := json.Unmarshal([]byte(test.body), &want); err != nil {
					t.Fatal(err)
				}
				if fake.updatedID != domain.UserID(test.target) || !reflect.DeepEqual(fake.updatedInput, want) {
					t.Fatalf("unexpected patch: id=%q input=%+v", fake.updatedID, fake.updatedInput)
				}
			}
			if strings.Contains(response.Body.String(), "new-password-456") || (test.status == http.StatusOK && strings.Contains(response.Body.String(), "password_hash")) {
				t.Fatal("patch response exposed credentials")
			}
		})
	}
}

func TestUpdateUserOpenAPIAndRemovedResetPassword(t *testing.T) {
	fake := &fakeAuth{}
	router := NewRouter(handler.NewAuthHandler(fake), handler.NewUserHandler(fake), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(`{}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("removed reset-password: status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil))
	var document struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					WriteOnly bool     `json:"writeOnly"`
					Enum      []string `json:"enum"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	request := document.Components.Schemas["UpdateUserRequest"]
	for _, field := range []string{"email", "password", "firstname", "lastname"} {
		if _, ok := request.Properties[field]; !ok {
			t.Errorf("PATCH schema is missing %q", field)
		}
	}
	if len(request.Required) != 0 || !request.Properties["password"].WriteOnly {
		t.Fatal("PATCH fields must be optional and password write-only")
	}
	for _, field := range []string{"id", "email", "firstname", "lastname", "created_at", "updated_at"} {
		if _, ok := document.Components.Schemas["UpdatedUserResponse"].Properties[field]; !ok {
			t.Errorf("PATCH response schema is missing %q", field)
		}
	}
	for _, name := range []string{"RegisterRequest", "RegisteredUserResponse", "UpdateUserRequest", "UpdatedUserResponse"} {
		if _, exists := document.Components.Schemas[name].Properties["role"]; exists {
			t.Errorf("user schema %q contains a global role", name)
		}
	}
	for _, removed := range []string{"reset-password", "resetPassword", "ResetPassword"} {
		if strings.Contains(response.Body.String(), removed) {
			t.Errorf("OpenAPI contains removed %q", removed)
		}
	}
}
