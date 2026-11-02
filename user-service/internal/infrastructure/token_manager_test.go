package infrastructure

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAccessTokenIdentifiesUserWithoutGlobalRole(t *testing.T) {
	manager := NewTokenManager("test-secret", "test")
	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	token, err := manager.CreateAccessToken("user-id", expiresAt)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if _, exists := claims["role"]; exists {
		t.Fatal("user token must not contain a global role")
	}
	principal, err := manager.ParseAccessToken(token)
	if err != nil || principal.UserID != "user-id" || !principal.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("token round trip: principal=%+v error=%v", principal, err)
	}
	if _, err := NewTokenManager("another-secret", "test").ParseAccessToken(token); err == nil {
		t.Fatal("token with an invalid signature was accepted")
	}
}
