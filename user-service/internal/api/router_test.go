package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jur4ikoff/user-service/internal/api/handler"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

const testUserID = "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1"

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
	if getResponse.Code != http.StatusNotImplemented {
		t.Fatalf("get user: got status %d, want %d; body: %s", getResponse.Code, http.StatusNotImplemented, getResponse.Body.String())
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

func (f *fakeAuth) Register(_ context.Context, input service.RegisterInput) (service.RegisteredUser, error) {
	f.registerInput = input
	f.registerCalls++
	return service.RegisteredUser{ID: testUserID, Email: input.Email, Firstname: input.Firstname, Lastname: input.Lastname}, f.registerErr
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
