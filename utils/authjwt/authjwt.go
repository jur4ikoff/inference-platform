// Package authjwt implements the small JWT/JWKS contract shared by platform
// services. It intentionally accepts only RS256 access tokens and never
// follows key URLs from an untrusted token.
package authjwt

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

const ClaimsVersion = 1

type Permissions struct {
	AllPermissions   bool `json:"all_permissions,omitempty"`
	CanReadProject   bool `json:"can_read_project,omitempty"`
	CanUpdateProject bool `json:"can_update_project,omitempty"`
	CanDeleteProject bool `json:"can_delete_project,omitempty"`
	CanManageMembers bool `json:"can_manage_members,omitempty"`
	CanCreateModel   bool `json:"can_create_model,omitempty"`
	CanReadModel     bool `json:"can_read_model,omitempty"`
	CanUpdateModel   bool `json:"can_update_model,omitempty"`
	CanDeleteModel   bool `json:"can_delete_model,omitempty"`
	CanStartModel    bool `json:"can_start_model,omitempty"`
	CanStopModel     bool `json:"can_stop_model,omitempty"`
}

type ProjectGrant struct {
	Name        string      `json:"name"`
	Role        string      `json:"role"`
	Permissions Permissions `json:"permissions"`
}

type Claims struct {
	Subject      string        `json:"sub"`
	Username     string        `json:"username"`
	Issuer       string        `json:"iss"`
	Audience     string        `json:"aud"`
	IssuedAt     int64         `json:"iat"`
	ExpiresAt    int64         `json:"exp"`
	AuthTime     int64         `json:"auth_time"`
	ID           string        `json:"jti"`
	AuthVersion  int64         `json:"auth_version"`
	PlatformRole string        `json:"platform_role"`
	Version      int           `json:"claims_version"`
	Project      *ProjectGrant `json:"project,omitempty"`
}

type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKSet struct {
	Keys []JWK `json:"keys"`
}

func PublicJWK(key *rsa.PublicKey, kid string) (JWK, error) {
	if key == nil || key.N == nil || key.Size() < 256 || strings.TrimSpace(kid) == "" {
		return JWK{}, errors.New("invalid RSA public key")
	}
	e := big.NewInt(int64(key.E)).Bytes()
	return JWK{Kty: "RSA", Use: "sig", Kid: kid, Alg: "RS256", N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()), E: base64.RawURLEncoding.EncodeToString(e)}, nil
}

func Sign(key *rsa.PrivateKey, kid string, claims Claims) (string, error) {
	if key == nil || key.N == nil || key.Size() < 256 || strings.TrimSpace(kid) == "" {
		return "", errors.New("invalid RSA signing key")
	}
	if claims.Version == 0 {
		claims.Version = ClaimsVersion
	}
	header, err := json.Marshal(struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
		KeyID     string `json:"kid"`
	}{Algorithm: "RS256", Type: "at+jwt", KeyID: kid})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func Verify(raw string, keys JWKSet, issuer, audience string, now time.Time, leeway time.Duration) (Claims, error) {
	if leeway < 0 || leeway > 5*time.Minute || len(raw) > 4096 {
		return Claims{}, errors.New("invalid token")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Claims{}, errors.New("invalid token")
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
		KeyID     string `json:"kid"`
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != "RS256" || header.Type != "at+jwt" || header.KeyID == "" {
		return Claims{}, errors.New("invalid token")
	}
	var public *rsa.PublicKey
	for _, key := range keys.Keys {
		if key.Kid != header.KeyID || key.Kty != "RSA" || key.Use != "sig" || key.Alg != "RS256" {
			continue
		}
		public, err = publicKey(key)
		if err != nil {
			return Claims{}, errors.New("invalid token")
		}
		break
	}
	if public == nil {
		return Claims{}, errors.New("invalid token")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, errors.New("invalid token")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(public, crypto.SHA256, digest[:], signature) != nil {
		return Claims{}, errors.New("invalid token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, errors.New("invalid token")
	}
	var claims Claims
	if json.Unmarshal(payload, &claims) != nil || !validClaims(claims, issuer, audience, now, leeway) {
		return Claims{}, errors.New("invalid token")
	}
	return claims, nil
}

func publicKey(key JWK) (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil || len(n) == 0 {
		return nil, errors.New("invalid key")
	}
	e, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil || len(e) == 0 {
		return nil, errors.New("invalid key")
	}
	exponent := new(big.Int).SetBytes(e)
	if !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > int64(^uint(0)>>1) {
		return nil, errors.New("invalid key")
	}
	public := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent.Int64())}
	if public.Size() < 256 {
		return nil, errors.New("invalid key")
	}
	return public, nil
}

func validClaims(c Claims, issuer, audience string, now time.Time, leeway time.Duration) bool {
	if _, err := uuid.Parse(c.Subject); err != nil || strings.TrimSpace(c.Username) == "" || c.Issuer != issuer || c.Audience != audience || c.Version != ClaimsVersion || c.AuthVersion < 0 || c.PlatformRole == "" || c.ID == "" {
		return false
	}
	if c.IssuedAt <= 0 || c.ExpiresAt <= c.IssuedAt || c.ExpiresAt-c.IssuedAt > int64((30*time.Minute).Seconds()) || c.AuthTime <= 0 || c.AuthTime > c.IssuedAt {
		return false
	}
	n := now.Unix()
	l := int64(leeway.Seconds())
	if c.IssuedAt > n+l || c.ExpiresAt <= n-l {
		return false
	}
	if c.Project != nil && (c.Project.Name == "" || c.Project.Role == "") {
		return false
	}
	return true
}
