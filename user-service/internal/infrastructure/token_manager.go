package infrastructure

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/utils/authjwt"
)

// TokenManager issues and validates RS256 access tokens. Only this service is
// given the private key; consumers use PublicJWKSet instead.
type TokenManager struct {
	private  *rsa.PrivateKey
	issuer   string
	audience string
	kid      string
	now      func() time.Time
}

func NewTokenManager(private *rsa.PrivateKey, kid, issuer, audience string) (*TokenManager, error) {
	if private == nil || private.N == nil || private.Size() < 256 || kid == "" || issuer == "" || audience == "" {
		return nil, errors.New("invalid JWT signing configuration")
	}
	return &TokenManager{private: private, kid: kid, issuer: issuer, audience: audience, now: time.Now}, nil
}

func LoadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("JWT private key is not PEM")
	}
	if key, parseErr := x509.ParsePKCS1PrivateKey(block.Bytes); parseErr == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid JWT private key")
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("JWT private key is not RSA")
	}
	return rsaKey, nil
}

func (m *TokenManager) CreateAccessToken(session service.Session, project *authjwt.ProjectGrant, expiresAt time.Time) (string, error) {
	now := m.now().UTC()
	return authjwt.Sign(m.private, m.kid, authjwt.Claims{
		Subject: string(session.UserID), Username: session.Username, Issuer: m.issuer, Audience: m.audience,
		IssuedAt: now.Unix(), ExpiresAt: expiresAt.UTC().Unix(), AuthTime: session.AuthTime.UTC().Unix(),
		ID: newTokenID(), AuthVersion: session.AuthVersion, PlatformRole: string(session.PlatformRole), Project: project,
	})
}

func (m *TokenManager) ParseAccessToken(token string) (service.Principal, error) {
	key, err := authjwt.PublicJWK(&m.private.PublicKey, m.kid)
	if err != nil {
		return service.Principal{}, err
	}
	claims, err := authjwt.Verify(token, authjwt.JWKSet{Keys: []authjwt.JWK{key}}, m.issuer, m.audience, m.now().UTC(), 30*time.Second)
	if err != nil {
		return service.Principal{}, err
	}
	return service.Principal{UserID: domain.UserID(claims.Subject), Username: claims.Username, PlatformRole: domain.PlatformRole(claims.PlatformRole), AuthVersion: claims.AuthVersion, AuthTime: time.Unix(claims.AuthTime, 0).UTC(), ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(), Project: claims.Project}, nil
}

func (m *TokenManager) PublicJWKSet() (authjwt.JWKSet, error) {
	key, err := authjwt.PublicJWK(&m.private.PublicKey, m.kid)
	if err != nil {
		return authjwt.JWKSet{}, err
	}
	return authjwt.JWKSet{Keys: []authjwt.JWK{key}}, nil
}

func newTokenID() string { return uuid.NewString() }
