package service

import (
	"context"
	"github.com/rs/zerolog/log"
	"strings"
	"time"
)

// ProjectTokenService mints a narrowly-scoped grant from current database
// rights; it never copies permissions from the parent token.
type ProjectTokenService struct {
	projects ProjectTokenRepository
	tokens   TokenManager
	ttl      time.Duration
	clock    Clock
}

func NewProjectTokenService(projects ProjectTokenRepository, tokens TokenManager, ttl time.Duration, clock Clock) *ProjectTokenService {
	return &ProjectTokenService{projects: projects, tokens: tokens, ttl: ttl, clock: clock}
}

func (s *ProjectTokenService) Create(ctx context.Context, projectName string) (_ AccessToken, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "ProjectTokenService").Err(resultErr).Msg("Create failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "ProjectTokenService").Msg("Create completed")
	}()
	principal, ok := PrincipalFromContext(ctx)
	if !ok || strings.TrimSpace(projectName) == "" {
		return AccessToken{}, ErrInvalidToken
	}
	grant, err := s.projects.ProjectTokenGrant(ctx, principal.UserID, projectName)
	if err != nil {
		return AccessToken{}, err
	}
	now := s.clock.Now().UTC()
	expiresAt := now.Add(s.ttl)
	if principal.ExpiresAt.Before(expiresAt) {
		expiresAt = principal.ExpiresAt
	}
	value, err := s.tokens.CreateAccessToken(Session{UserID: principal.UserID, Username: principal.Username, PlatformRole: principal.PlatformRole, AuthVersion: principal.AuthVersion, AuthTime: principal.AuthTime}, &grant, expiresAt)
	if err != nil {
		return AccessToken{}, err
	}
	return AccessToken{Value: value, ExpiresAt: expiresAt}, nil
}
