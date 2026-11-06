package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/jur4ikoff/utils/authjwt"
)

const maxJWKSBytes = 1 << 20

type JWKSVerifier struct {
	url, issuer, audience string
	client                *http.Client
	ttl                   time.Duration
	now                   func() time.Time

	mu        sync.Mutex
	keys      authjwt.JWKSet
	fetchedAt time.Time
}

func NewJWKSVerifier(url, issuer, audience string, client *http.Client, ttl time.Duration) (*JWKSVerifier, error) {
	if url == "" || issuer == "" || audience == "" || client == nil || ttl <= 0 {
		return nil, errors.New("invalid JWKS verifier configuration")
	}
	return &JWKSVerifier{url: url, issuer: issuer, audience: audience, client: client, ttl: ttl, now: time.Now}, nil
}

func (v *JWKSVerifier) Verify(ctx context.Context, raw string) (Principal, error) {
	keys, err := v.keySet(ctx)
	if err != nil {
		return Principal{}, err
	}
	claims, err := authjwt.Verify(raw, keys, v.issuer, v.audience, v.now().UTC(), 30*time.Second)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	return Principal{Subject: claims.Subject, Username: claims.Username, PlatformRole: claims.PlatformRole, Project: claims.Project}, nil
}

func (v *JWKSVerifier) keySet(ctx context.Context) (authjwt.JWKSet, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.keys.Keys) != 0 && v.now().Sub(v.fetchedAt) < v.ttl {
		return v.keys, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return authjwt.JWKSet{}, fmt.Errorf("%w: build request", ErrUnavailable)
	}
	response, err := v.client.Do(request)
	if err != nil {
		return authjwt.JWKSet{}, fmt.Errorf("%w: fetch JWKS", ErrUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return authjwt.JWKSet{}, fmt.Errorf("%w: JWKS returned %d", ErrUnavailable, response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxJWKSBytes))
	var keys authjwt.JWKSet
	if err := decoder.Decode(&keys); err != nil || len(keys.Keys) == 0 {
		return authjwt.JWKSet{}, fmt.Errorf("%w: invalid JWKS", ErrUnavailable)
	}
	v.keys, v.fetchedAt = keys, v.now()
	return keys, nil
}
