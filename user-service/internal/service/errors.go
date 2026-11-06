package service

import (
	"errors"
	"math"
)

var (
	ErrForbidden          = errors.New("forbidden")
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
)

// normalizePageLimit is shared by the application list use cases.
func normalizePageLimit(offset, limit int) (int, error) {
	if limit == 0 {
		limit = 20
	}
	if offset < 0 || limit < 1 || limit > 100 || offset > math.MaxInt32-limit {
		return 0, ErrInvalidInput
	}
	return limit, nil
}
