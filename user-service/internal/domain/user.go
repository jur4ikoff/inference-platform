package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type UserID string

// ParseUserID validates the UUID used at the boundary of user operations.
func ParseUserID(value string) (UserID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return "", err
	}
	if id == uuid.Nil {
		return "", errors.New("user id must not be zero")
	}
	return UserID(id.String()), nil
}

type UserProfile struct {
	ID                         UserID
	Email, Firstname, Lastname string
	CreatedAt, UpdatedAt       time.Time
}
type Credentials struct {
	UserID              UserID
	Email, PasswordHash string
}
type AuthState struct {
	UserID       UserID
	PlatformRole PlatformRole
	AuthVersion  int64
}

var (
	ErrLastAdministrator    = errors.New("cannot delete last administrator")
	ErrLastTeamlead         = errors.New("cannot delete last project teamlead")
	ErrUserNotFound         = errors.New("user not found")
	ErrEmailAlreadyTaken    = errors.New("email already taken")
	ErrLoginAlreadyTaken    = errors.New("login already taken")
	ErrBootstrapAdminExists = errors.New("an administrator already exists")
)

type User struct {
	Firstname    string
	Lastname     string
	ID           UserID
	Email        string
	Login        string
	PasswordHash string
	PlatformRole PlatformRole
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
