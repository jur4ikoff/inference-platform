package service

import (
	"context"
	"time"

	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/utils/authjwt"
)

type TokenManager interface {
	CreateAccessToken(Session, *authjwt.ProjectGrant, time.Time) (string, error)
	ParseAccessToken(token string) (Principal, error)
}

type Principal struct {
	UserID       domain.UserID
	Username     string
	PlatformRole domain.PlatformRole
	AuthVersion  int64
	AuthTime     time.Time
	ExpiresAt    time.Time
	Project      *authjwt.ProjectGrant
}

type Session struct {
	UserID       domain.UserID
	Username     string
	PlatformRole domain.PlatformRole
	AuthVersion  int64
	AuthTime     time.Time
}

type principalKey struct{}

// WithPrincipal attaches a verified identity for user-facing operations.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
