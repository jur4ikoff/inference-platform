package domain

import (
	"errors"
	"time"
)

type UserID string

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrEmailAlreadyTaken = errors.New("email already taken")
)

type User struct {
	Firstname    string
	Lastname     string
	ID           UserID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserUpdate contains the fields supplied for a partial user update.
type UserUpdate struct {
	Email        *string
	PasswordHash *string
	Firstname    *string
	Lastname     *string
}
