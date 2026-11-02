package service

import "github.com/jur4ikoff/user-service/internal/domain"

type IDGenerator interface {
	NewUserID() domain.UserID
}
