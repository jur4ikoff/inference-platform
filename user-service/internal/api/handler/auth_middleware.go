package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jur4ikoff/user-service/internal/service"
)

type principalContextKey struct{}
type AuthMiddleware func(huma.Context, func(huma.Context))

type tokenVerifier interface {
	Verify(context.Context, string) (service.Principal, error)
}

// AuthenticationMiddleware validates a bearer token and makes the principal
// available to protected handlers through the request context.
func AuthenticationMiddleware(api huma.API, auth tokenVerifier) AuthMiddleware {
	return func(ctx huma.Context, next func(huma.Context)) {
		rawToken, ok := bearerToken(ctx.Header("Authorization"))
		if !ok {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}

		principal, err := auth.Verify(ctx.Context(), rawToken)
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}

		next(huma.WithValue(ctx, principalContextKey{}, principal))
	}
}

// PrincipalFromContext returns the authenticated principal set by
// AuthenticationMiddleware.
func PrincipalFromContext(ctx context.Context) (service.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(service.Principal)
	return principal, ok
}

func bearerToken(authorization string) (string, bool) {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
