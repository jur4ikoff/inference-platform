package infrastructure

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

// TokenManager creates and validates compact HS256 JWT access tokens.
type TokenManager struct {
	secret []byte
	issuer string
	now    func() time.Time
}

func NewTokenManager(secret, issuer string) *TokenManager {
	return &TokenManager{secret: []byte(secret), issuer: issuer, now: time.Now}
}

func (m *TokenManager) CreateAccessToken(userID domain.UserID, expiresAt time.Time) (string, error) {
	header, err := json.Marshal(struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}{Algorithm: "HS256", Type: "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		Subject string `json:"sub"`
		Issuer  string `json:"iss"`
		Expires int64  `json:"exp"`
	}{Subject: string(userID), Issuer: m.issuer, Expires: expiresAt.Unix()})
	if err != nil {
		return "", err
	}

	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(m.sign(unsigned)), nil
}

func (m *TokenManager) ParseAccessToken(token string) (service.Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return service.Principal{}, errors.New("invalid token")
	}
	providedSignature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(providedSignature, m.sign(parts[0]+"."+parts[1])) {
		return service.Principal{}, errors.New("invalid token")
	}

	decodedHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return service.Principal{}, errors.New("invalid token")
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if json.Unmarshal(decodedHeader, &header) != nil || header.Algorithm != "HS256" || header.Type != "JWT" {
		return service.Principal{}, errors.New("invalid token")
	}

	decodedPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return service.Principal{}, errors.New("invalid token")
	}
	var claims struct {
		Subject string `json:"sub"`
		Issuer  string `json:"iss"`
		Expires int64  `json:"exp"`
	}
	if json.Unmarshal(decodedPayload, &claims) != nil ||
		claims.Subject == "" || claims.Issuer != m.issuer ||
		claims.Expires <= m.now().Unix() {
		return service.Principal{}, errors.New("invalid token")
	}

	return service.Principal{
		UserID:    domain.UserID(claims.Subject),
		ExpiresAt: time.Unix(claims.Expires, 0).UTC(),
	}, nil
}

func (m *TokenManager) sign(value string) []byte {
	hash := hmac.New(sha256.New, m.secret)
	_, _ = hash.Write([]byte(value))
	return hash.Sum(nil)
}
