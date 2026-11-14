package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jur4ikoff/model-service/internal/auth"
	"github.com/jur4ikoff/model-service/internal/service"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/metadata"
)

type principalContextKey struct{}

func principalFromContext(ctx context.Context) (auth.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(auth.Principal)
	return principal, ok
}

func (h *Handler) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, apiPrefix+"/projects/") {
			next.ServeHTTP(w, r)
			return
		}
		raw, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeAuthError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}
		principal, err := h.auth.Verify(r.Context(), raw)
		if err != nil {
			if errors.Is(err, auth.ErrUnavailable) {
				writeAuthError(w, http.StatusServiceUnavailable, "authentication service is unavailable")
			} else {
				writeAuthError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			}
			return
		}
		project, action, ok := protectedAction(r)
		if !ok || !allowed(principal, project, action) {
			writeAuthError(w, http.StatusForbidden, "access denied")
			return
		}
		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		ctx = service.WithActor(ctx, principal.Username)
		logger := log.Ctx(ctx).With().Str("username", principal.Username).Str("project", project).Logger()
		ctx = logger.WithContext(ctx)
		ctx = metadata.AppendToOutgoingContext(ctx, "x-log-username", principal.Username, "x-log-project", project)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func protectedAction(r *http.Request) (project, action string, ok bool) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "projects" {
		return "", "", false
	}
	project = parts[3]
	if parts[4] == "deployments" && r.Method == http.MethodGet && (len(parts) == 5 || (len(parts) == 6 && parts[5] == "history")) {
		return project, "read", true
	}
	if parts[4] != "models" {
		return "", "", false
	}
	deployment := len(parts) == 7 && parts[6] == "deployment"
	switch r.Method {
	case http.MethodGet:
		return project, "read", true
	case http.MethodPost:
		if deployment {
			return project, "start", true
		}
		return project, "create", true
	case http.MethodPatch:
		return project, "update", true
	case http.MethodDelete:
		if deployment {
			return project, "stop", true
		}
		return project, "delete", true
	default:
		return "", "", false
	}
}

func allowed(principal auth.Principal, project, action string) bool {
	if principal.PlatformRole == "admin" {
		return true
	}
	if action == "read" && principal.PlatformRole == "analyst" {
		return true
	}
	if principal.Project == nil || principal.Project.Name != project {
		return false
	}
	p := principal.Project.Permissions
	if p.AllPermissions {
		return true
	}
	switch action {
	case "create":
		return p.CanCreateModel
	case "read":
		return p.CanReadModel
	case "update":
		return p.CanUpdateModel
	case "delete":
		return p.CanDeleteModel
	case "start":
		return p.CanStartModel
	case "stop":
		return p.CanStopModel
	}
	return false
}

func bearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	return func() (string, bool) {
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			return "", false
		}
		return parts[1], true
	}()
}

func writeAuthError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}{Status: status, Detail: detail})
}
