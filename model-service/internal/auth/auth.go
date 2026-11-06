// Package auth implements authentication at the ModelService boundary.
package auth

import (
	"context"
	"errors"

	"github.com/jur4ikoff/utils/authjwt"
)

var (
	ErrInvalidToken = errors.New("invalid access token")
	ErrUnavailable  = errors.New("JWT verifier is unavailable")
)

type Principal struct {
	Subject      string
	Username     string
	PlatformRole string
	Project      *authjwt.ProjectGrant
}

type Verifier interface {
	Verify(context.Context, string) (Principal, error)
}
