package service

import (
	"time"

	"github.com/jur4ikoff/user-service/internal/domain"
)

type TokenManager interface {
	CreateAccessToken(userID domain.UserID, expiresAt time.Time) (string, error)
	ParseAccessToken(token string) (Principal, error)
}

type Principal struct {
	UserID    domain.UserID
	ExpiresAt time.Time
}
