package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/utils/authjwt"
)

// Техника: классы эквивалентности корректной и некорректной подписи токена.
func TestAccessTokenIsIndependentlyVerifiedByPublicJWK(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		private, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			allureCtx.Fatal(err)
		}
		manager, err := NewTokenManager(private, "test-key", "test", "platform")
		if err != nil {
			allureCtx.Fatal(err)
		}
		expiresAt := time.Now().UTC().Add(time.Minute)
		userID := domain.UserID("0197eb5a-37c6-7e48-8cd6-0c19edcd72f1")
		token, err := manager.CreateAccessToken(service.Session{UserID: userID, Username: "alice", PlatformRole: domain.PlatformRoleUser, AuthTime: time.Now().UTC()}, nil, expiresAt)
		if err != nil {
			allureCtx.Fatal(err)
		}
		keys, err := manager.PublicJWKSet()
		if err != nil {
			allureCtx.Fatal(err)
		}
		claims, err := authjwt.Verify(token, keys, "test", "platform", time.Now().UTC(), 0)
		if err != nil || claims.Project != nil || claims.PlatformRole != "user" || claims.Username != "alice" {
			allureCtx.Fatalf("independent verification: claims=%+v error=%v", claims, err)
		}
		principal, err := manager.ParseAccessToken(token)
		if err != nil || principal.UserID != userID || principal.Username != "alice" || principal.PlatformRole != domain.PlatformRoleUser {
			allureCtx.Fatalf("token round trip: principal=%+v error=%v", principal, err)
		}
		projectTokens := service.NewProjectTokenService(testProjectGrant{}, manager, time.Minute, testTokenClock{})
		projectToken, err := projectTokens.Create(service.WithPrincipal(context.Background(), principal), "demo")
		if err != nil {
			allureCtx.Fatal(err)
		}
		projectClaims, err := authjwt.Verify(projectToken.Value, keys, "test", "platform", time.Now().UTC(), 0)
		if err != nil || projectClaims.Username != "alice" || projectClaims.Project == nil || projectClaims.Project.Name != "demo" {
			allureCtx.Fatalf("project token: claims=%+v error=%v", projectClaims, err)
		}
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			allureCtx.Fatal(err)
		}
		other, err := NewTokenManager(otherKey, "other-key", "test", "platform")
		if err != nil {
			allureCtx.Fatal(err)
		}
		if _, err := other.ParseAccessToken(token); err == nil {
			allureCtx.Fatal("token with an invalid signature was accepted")
		}
	})
}

type testProjectGrant struct{}

func (testProjectGrant) ProjectTokenGrant(_ context.Context, _ domain.UserID, name string) (authjwt.ProjectGrant, error) {
	return authjwt.ProjectGrant{Name: name, Role: "developer"}, nil
}

type testTokenClock struct{}

func (testTokenClock) Now() time.Time { return time.Now() }
