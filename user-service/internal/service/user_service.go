package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/jur4ikoff/user-service/internal/domain"
)

const (
	minimumPasswordLength = 12
	maximumBcryptLength   = 72
)

// UserService contains application use cases. It does not know about HTTP,
// SQL, bcrypt, or JWT implementations.
type UserService struct {
	users     UserRepository
	hasher    PasswordHasher
	tokens    TokenManager
	ids       IDGenerator
	clock     Clock
	accessTTL time.Duration
}

type RegisterInput struct {
	Firstname string
	Lastname  string
	Email     string
	Password  string
}

type RegisteredUser struct {
	ID        domain.UserID
	Email     string
	Firstname string
	Lastname  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type LoginInput struct {
	Email    string
	Password string
}

type AccessToken struct {
	Value     string
	ExpiresAt time.Time
}

// UpdateUserInput contains only fields that a user is allowed to change.
// A nil field means it was omitted from the PATCH request.
type UpdateUserInput struct {
	Firstname *string
	Lastname  *string
	Email     *string
	Password  *string
}

func NewUserService(
	users UserRepository,
	hasher PasswordHasher,
	tokens TokenManager,
	ids IDGenerator,
	clock Clock,
	accessTTL time.Duration,
) *UserService {
	return &UserService{
		users:     users,
		hasher:    hasher,
		tokens:    tokens,
		ids:       ids,
		clock:     clock,
		accessTTL: accessTTL,
	}
}

func (s *UserService) Register(ctx context.Context, input RegisterInput) (RegisteredUser, error) {
	firstname, lastname := strings.TrimSpace(input.Firstname), strings.TrimSpace(input.Lastname)
	email, err := normalizeEmail(input.Email)
	if err != nil || firstname == "" || lastname == "" || !validPassword(input.Password) {
		return RegisteredUser{}, ErrInvalidInput
	}

	hash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return RegisteredUser{}, err
	}

	now := s.clock.Now().UTC()
	user := domain.User{
		ID:           s.ids.NewUserID(),
		Firstname:    firstname,
		Lastname:     lastname,
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.users.Create(ctx, user); err != nil {
		return RegisteredUser{}, err
	}

	return RegisteredUser{
		ID: user.ID, Email: user.Email,
		Firstname: user.Firstname, Lastname: user.Lastname,
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}, nil
}

func (s *UserService) Login(ctx context.Context, input LoginInput) (AccessToken, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return AccessToken{}, ErrInvalidCredentials
	}

	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			// Equalise the most visible timing difference between an existing and
			// non-existing account. Never expose which part of credentials failed.
			_ = s.hasher.Compare(dummyPasswordHash, input.Password)
			return AccessToken{}, ErrInvalidCredentials
		}
		return AccessToken{}, err
	}

	if err := s.hasher.Compare(user.PasswordHash, input.Password); err != nil {
		return AccessToken{}, ErrInvalidCredentials
	}

	expiresAt := s.clock.Now().UTC().Add(s.accessTTL)
	value, err := s.tokens.CreateAccessToken(user.ID, expiresAt)
	if err != nil {
		return AccessToken{}, err
	}

	return AccessToken{Value: value, ExpiresAt: expiresAt}, nil
}

func (s *UserService) Verify(_ context.Context, rawToken string) (Principal, error) {
	if strings.TrimSpace(rawToken) == "" {
		return Principal{}, ErrInvalidToken
	}

	principal, err := s.tokens.ParseAccessToken(rawToken)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	return principal, nil
}

func (s *UserService) UpdateUser(ctx context.Context, id domain.UserID, input UpdateUserInput) (RegisteredUser, error) {
	if input.Email == nil && input.Password == nil && input.Firstname == nil && input.Lastname == nil {
		return RegisteredUser{}, ErrInvalidInput
	}

	if input.Firstname != nil {
		firstname := strings.TrimSpace(*input.Firstname)
		input.Firstname = &firstname
	}
	if input.Lastname != nil {
		lastname := strings.TrimSpace(*input.Lastname)
		input.Lastname = &lastname
	}

	var email *string
	if input.Email != nil {
		normalized, err := normalizeEmail(*input.Email)
		if err != nil {
			return RegisteredUser{}, ErrInvalidInput
		}
		email = &normalized
	}

	var passwordHash *string
	if input.Password != nil {
		if !validPassword(*input.Password) {
			return RegisteredUser{}, ErrInvalidInput
		}
		hash, err := s.hasher.Hash(*input.Password)
		if err != nil {
			return RegisteredUser{}, err
		}
		passwordHash = &hash
	}

	user, err := s.users.Update(ctx, id, domain.UserUpdate{
		Email:        email,
		PasswordHash: passwordHash,
		Firstname:    input.Firstname,
		Lastname:     input.Lastname,
	}, s.clock.Now().UTC())
	if err != nil {
		return RegisteredUser{}, err
	}
	return RegisteredUser{
		ID: user.ID, Email: user.Email,
		Firstname: user.Firstname, Lastname: user.Lastname,
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}, nil
}

func (s *UserService) DeleteUser(ctx context.Context, id domain.UserID) error {
	return s.users.Delete(ctx, id)
}

type ListUsersInput struct {
	Offset int
	Limit  int
}

type UsersPage struct {
	Users         []RegisteredUser
	NextPageToken *int
}

func (s *UserService) ListUsers(ctx context.Context, input ListUsersInput) (UsersPage, error) {
	limit, err := normalizePageLimit(input.Offset, input.Limit)
	if err != nil {
		return UsersPage{}, err
	}
	input.Limit = limit
	// One extra row tells us whether another page exists without a count query.
	users, err := s.users.List(ctx, input.Offset, input.Limit+1)
	if err != nil {
		return UsersPage{}, err
	}
	page := UsersPage{}
	if len(users) > input.Limit {
		nextOffset := input.Offset + input.Limit
		page.NextPageToken = &nextOffset
		users = users[:input.Limit]
	}
	result := make([]RegisteredUser, 0, len(users))
	for _, user := range users {
		result = append(result, RegisteredUser{ID: user.ID, Email: user.Email, Firstname: user.Firstname, Lastname: user.Lastname})
	}
	page.Users = result
	return page, nil
}

func normalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if len(email) == 0 || len(email) > 254 {
		return "", ErrInvalidInput
	}

	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", ErrInvalidInput
	}
	return email, nil
}

func validPassword(password string) bool {
	return len(password) >= minimumPasswordLength && len(password) <= maximumBcryptLength
}

// This is a bcrypt hash of an arbitrary value. It is used only to reduce
// account-enumeration timing differences when the account does not exist.
const dummyPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
