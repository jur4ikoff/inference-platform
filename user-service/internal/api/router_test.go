package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jur4ikoff/user-service/internal/api/handler"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/utils/authjwt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const testUserID = "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1"

// Техника: классы эквивалентности авторизованных и анонимных запросов.
func TestUserRoutesRequireBearerTokenAndAllowOwner(t *testing.T) {
	fake := &fakeAuth{principal: service.Principal{UserID: testUserID}}
	router := NewRouter(
		handler.NewAuthHandler(fake),
		handler.NewUserHandler(fake),
		handler.NewProjectHandler(nil),
		handler.NewRoleHandler(nil),
		"test",
	)

	unauthorized := httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+testUserID, nil)
	unauthorizedResponse := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: got status %d, want %d", unauthorizedResponse.Code, http.StatusUnauthorized)
	}

	patch := httptest.NewRequest(http.MethodPatch, "/api/v1/users/"+testUserID, bytes.NewBufferString(`{"email":"new@example.com"}`))
	patch.Header.Set("Authorization", "Bearer access-token")
	patch.Header.Set("Content-Type", "application/json")
	patchResponse := httptest.NewRecorder()
	router.ServeHTTP(patchResponse, patch)
	if patchResponse.Code != http.StatusOK {
		t.Fatalf("patch: got status %d, want %d; body: %s", patchResponse.Code, http.StatusOK, patchResponse.Body.String())
	}
	if fake.updatedID != testUserID || fake.updatedEmail == nil || *fake.updatedEmail != "new@example.com" {
		t.Fatalf("unexpected update input: id=%q email=%v", fake.updatedID, fake.updatedEmail)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+testUserID, nil)
	deleteRequest.Header.Set("Authorization", "Bearer access-token")
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete: got status %d, want %d; body: %s", deleteResponse.Code, http.StatusNoContent, deleteResponse.Body.String())
	}
	if fake.deletedID != testUserID {
		t.Fatalf("delete used id %q, want %q", fake.deletedID, testUserID)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+testUserID, nil)
	getRequest.Header.Set("Authorization", "Bearer access-token")
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get user: got status %d, want %d; body: %s", getResponse.Code, http.StatusOK, getResponse.Body.String())
	}
}

type fakeAuth struct {
	principal     service.Principal
	updatedID     domain.UserID
	updatedEmail  *string
	updatedInput  service.UpdateUserInput
	updateCalls   int
	deletedID     domain.UserID
	registerInput service.RegisterInput
	registerCalls int
	registerErr   error
	users         []service.RegisteredUser
	listCalls     int
	listErr       error
	listInput     service.ListUsersInput
	nextPageToken *int
	verifyErr     error
}

func (f *fakeAuth) GetUser(_ context.Context, id domain.UserID) (service.RegisteredUser, error) {
	return service.RegisteredUser{ID: id}, nil
}

func (f *fakeAuth) GetUserByLogin(_ context.Context, login string) (service.RegisteredUser, error) {
	return service.RegisteredUser{ID: testUserID, Login: login}, nil
}

func (f *fakeAuth) Register(_ context.Context, input service.RegisterInput) (service.RegisteredUser, error) {
	f.registerInput = input
	f.registerCalls++
	return service.RegisteredUser{ID: testUserID, Login: input.Login, Email: input.Email, Firstname: input.Firstname, Lastname: input.Lastname}, f.registerErr
}

func (f *fakeAuth) ListUsers(_ context.Context, input service.ListUsersInput) (service.UsersPage, error) {
	f.listCalls++
	f.listInput = input
	return service.UsersPage{Users: f.users, NextPageToken: f.nextPageToken}, f.listErr
}

func (f *fakeAuth) Login(context.Context, service.LoginInput) (service.AccessToken, error) {
	return service.AccessToken{}, nil
}

func (f *fakeAuth) Verify(context.Context, string) (service.Principal, error) {
	return f.principal, f.verifyErr
}

func (*fakeAuth) PublicJWKSet() (authjwt.JWKSet, error) { return authjwt.JWKSet{}, nil }

func (f *fakeAuth) AuthorizeUser(_ context.Context, target domain.UserID) error {
	if f.principal.UserID != target {
		return service.ErrForbidden
	}
	return nil
}

// Техника: классы эквивалентности результатов проверки токена.
func TestInternalAuthentication(t *testing.T) {
	for _, test := range []struct {
		name, authorization string
		err                 error
		status              int
	}{
		{"missing token", "", nil, http.StatusUnauthorized},
		{"wrong scheme", "Basic token", nil, http.StatusUnauthorized},
		{"invalid token", "Bearer invalid", service.ErrInvalidToken, http.StatusUnauthorized},
		{"valid token", "Bearer access-token", nil, http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeAuth{principal: service.Principal{UserID: testUserID}, verifyErr: test.err}
			router := NewRouter(handler.NewAuthHandler(fake), handler.NewUserHandler(fake), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
			request := httptest.NewRequest(http.MethodGet, "/internal/auth", nil)
			request.Header.Set("Authorization", test.authorization)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if test.status == http.StatusNoContent {
				if response.Header().Get("X-User-ID") != testUserID || response.Header().Get("X-User-Role") != "" || response.Body.Len() != 0 {
					t.Fatal("internal authentication must return the principal headers and an empty body")
				}
			} else if response.Header().Get("X-User-ID") != "" || response.Header().Get("X-User-Role") != "" {
				t.Fatal("failed authentication returned principal headers")
			}
		})
	}
}

// Техника: класс удалённого маршрута.
func TestVerifyRemovedFromPublicAPI(t *testing.T) {
	fake := &fakeAuth{}
	router := NewRouter(handler.NewAuthHandler(fake), handler.NewUserHandler(fake), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("removed endpoint: status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("OpenAPI: status = %d", response.Code)
	}
	for _, removed := range []string{"/auth/verify", "verifyAccessToken", "VerifyInput", "VerifyOutput", "/internal/auth"} {
		if strings.Contains(response.Body.String(), removed) {
			t.Errorf("public OpenAPI contains %q", removed)
		}
	}
}

func (f *fakeAuth) UpdateUser(_ context.Context, id domain.UserID, input service.UpdateUserInput) (service.RegisteredUser, error) {
	f.updatedID = id
	f.updatedEmail = input.Email
	f.updatedInput = input
	f.updateCalls++
	return service.RegisteredUser{ID: id, Email: "new@example.com"}, nil
}

func (f *fakeAuth) DeleteUser(_ context.Context, id domain.UserID) error {
	f.deletedID = id
	return nil
}

// Техника: класс версионированной документации.
func TestVersionedDocumentation(t *testing.T) {
	fake := &fakeAuth{}
	router := NewRouter(handler.NewAuthHandler(fake), handler.NewUserHandler(fake), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
	for _, path := range []string{"/api/v1/docs", "/api/v1/openapi.yaml", "/api/v1/openapi.json"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d", path, response.Code)
		}
		if path == "/api/v1/docs" && !strings.Contains(response.Body.String(), "/api/v1/openapi.json") {
			t.Error("Swagger must load the versioned OpenAPI document")
		}
		if path == "/api/v1/openapi.json" && strings.Contains(response.Body.String(), `"$schema"`) {
			t.Error("OpenAPI contains automatic $schema fields")
		}
	}
	for _, path := range []string{"/users/" + testUserID, "/auth/login", "/docs", "/openapi.yaml", "/api/v2/users/" + testUserID} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, response.Code)
		}
	}
}

// Техника: классы эквивалентности данных регистрации и ошибок.
func TestRegistrationRoutes(t *testing.T) {
	const path = "/api/v1/auth/register"
	for _, tt := range []struct {
		name   string
		body   string
		err    error
		status int
		calls  int
	}{
		{"success without token", `{"login":"user","firstname":"Юрий","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, nil, http.StatusCreated, 1},
		{"duplicate email", `{"login":"user","firstname":"Юрий","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, domain.ErrEmailAlreadyTaken, http.StatusConflict, 1},
		{"invalid input", `{"login":"user","firstname":"Юрий","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, service.ErrInvalidInput, http.StatusBadRequest, 1},
		{"storage error", `{"login":"user","firstname":"Юрий","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, errors.New("private database error"), http.StatusInternalServerError, 1},
		{"invalid email", `{"login":"user","firstname":"Юрий","lastname":"Попов","email":"invalid","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"short password", `{"login":"user","firstname":"Юрий","lastname":"Попов","email":"user@example.com","password":"short"}`, nil, http.StatusUnprocessableEntity, 0},
		{"missing password", `{"login":"user","firstname":"Юрий","lastname":"Попов","email":"user@example.com"}`, nil, http.StatusUnprocessableEntity, 0},
		{"missing email", `{"login":"user","firstname":"Юрий","lastname":"Попов","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"missing firstname", `{"login":"user","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"missing lastname", `{"login":"user","firstname":"Юрий","email":"user@example.com","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"empty firstname", `{"login":"user","firstname":"","lastname":"Попов","email":"user@example.com","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"blank lastname", `{"login":"user","firstname":"Юрий","lastname":"   ","email":"user@example.com","password":"password-12345"}`, nil, http.StatusUnprocessableEntity, 0},
		{"role is not a user field", `{"login":"user","firstname":"Юрий","lastname":"Попов","email":"user@example.com","password":"password-12345","role":"admin"}`, nil, http.StatusUnprocessableEntity, 0},
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
			if tt.calls > 0 && fake.registerInput != (service.RegisterInput{Login: "user", Firstname: "Юрий", Lastname: "Попов", Email: "user@example.com", Password: "password-12345"}) {
				t.Fatal("registration body was not passed to service")
			}
			if tt.status == http.StatusCreated {
				var user map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &user); err != nil {
					t.Fatal(err)
				}
				if len(user) != 5 || user["login"] != "user" || user["id"] != testUserID || user["email"] != "user@example.com" || user["firstname"] != "Юрий" || user["lastname"] != "Попов" {
					t.Fatalf("unexpected registration response: %v", user)
				}
			}
			if strings.Contains(response.Body.String(), "private database error") || strings.Contains(response.Body.String(), "password-12345") {
				t.Fatal("response exposed password or internal error")
			}
		})
	}
}

// Техника: классы существующих и удалённых маршрутов.
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
	if len(schema.Required) != 5 || !schema.Properties["password"].WriteOnly {
		t.Fatal("registration must document all five required fields and a write-only password")
	}
	for _, field := range []string{"login", "firstname", "lastname", "email", "password"} {
		found := false
		for _, required := range schema.Required {
			found = found || required == field
		}
		if !found {
			t.Errorf("registration field %q is not required", field)
		}
	}
}

// Техника: классы эквивалентности ответов списка.
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

// Техника: граничные значения пагинации.
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

// Техника: классы эквивалентности прав доступа.
func TestUpdateUserProfilePermissions(t *testing.T) {
	const otherID = "0197eb5a-37c6-7e48-8cd6-0c19edcd72f2"
	for _, test := range []struct {
		name, token, target, body string
		status, calls             int
	}{
		{"owner profile", "Bearer token", testUserID, `{"firstname":"Юрий","lastname":"Попов"}`, http.StatusOK, 1},
		{"owner password", "Bearer token", testUserID, `{"password":"new-password-456"}`, http.StatusOK, 1},
		{"no token", "", testUserID, `{"firstname":"Юрий"}`, http.StatusUnauthorized, 0},
		{"another user", "Bearer token", otherID, `{"firstname":"Юрий"}`, http.StatusForbidden, 0},
		{"self promotion", "Bearer token", testUserID, `{"role":"admin","firstname":"Юрий"}`, http.StatusUnprocessableEntity, 0},
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

// Техника: классы существующих и удалённых маршрутов.
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

// Техника: граничные значения длины пароля.
func TestRegistrationPasswordLength(t *testing.T) {
	for _, tc := range []struct {
		password string
		status   int
	}{{"1234567", http.StatusUnprocessableEntity}, {"12345678", http.StatusCreated}} {
		fake := &fakeAuth{}
		router := NewRouter(handler.NewAuthHandler(fake), handler.NewUserHandler(fake), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
		body := `{"login":"alice","firstname":"Alice","lastname":"Test","email":"alice@example.com","password":"` + tc.password + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != tc.status {
			t.Errorf("password %q: status=%d, body=%s", tc.password, response.Code, response.Body.String())
		}
	}
}

// Техника: граничные значения пагинации.
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

// Техника: классы эквивалентности путей и идентификаторов.
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
	listRequest(t, router, http.MethodGet, "/api/v1/projects?limit=0", "", true, http.StatusUnprocessableEntity)
	listRequest(t, router, http.MethodGet, "/api/v1/projects", "", false, http.StatusUnauthorized)
}

// Техника: граничные значения параметров пагинации.
func TestUserServicePaginationOpenAPI(t *testing.T) {
	auth := &fakeAuth{}
	router := NewRouter(handler.NewAuthHandler(auth), handler.NewUserHandler(auth), handler.NewProjectHandler(nil), handler.NewRoleHandler(nil), "test")
	response := listRequest(t, router, http.MethodGet, "/api/v1/openapi.json", "", false, http.StatusOK)
	var document struct {
		Paths map[string]map[string]struct {
			Responses  map[string]json.RawMessage `json:"responses"`
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
		params := document.Paths["/api/v1/projects/{project_name}"][method].Parameters
		if len(params) != 1 || params[0].Name != "project_name" || params[0].In != "path" || !params[0].Required {
			t.Errorf("project %s must use required name path parameter", method)
		}
	}
	if _, exists := document.Paths["/api/v1/projects/list"]; exists {
		t.Fatal("old project list route is still documented")
	}
	responses := document.Paths["/api/v1/projects/{project_name}/members/{user_id}"]["put"].Responses
	for _, code := range []string{"200", "201", "403", "404", "409"} {
		if _, exists := responses[code]; !exists {
			t.Errorf("member PUT response %s missing from OpenAPI", code)
		}
	}
	if _, exists := responses["204"]; exists {
		t.Error("member PUT must not advertise 204")
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
	service.RoleRepository
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
