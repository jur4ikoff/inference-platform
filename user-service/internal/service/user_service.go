package service

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/utils/authjwt"
	"github.com/rs/zerolog/log"
)

const (
	minimumPasswordLength = 8
	maximumBcryptLength   = 72
	maximumAccessTokenTTL = 30 * time.Minute
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
	Login     string
	Firstname string
	Lastname  string
	Email     string
	Password  string
}

type RegisteredUser struct {
	Login        string
	PlatformRole string
	ID           domain.UserID
	Email        string
	Firstname    string
	Lastname     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type LoginInput struct {
	Login    string
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
	if accessTTL <= 0 || accessTTL > maximumAccessTokenTTL {
		accessTTL = maximumAccessTokenTTL
	}
	return &UserService{
		users:     users,
		hasher:    hasher,
		tokens:    tokens,
		ids:       ids,
		clock:     clock,
		accessTTL: accessTTL,
	}
}

func (s *UserService) Register(ctx context.Context, input RegisterInput) (_ RegisteredUser, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "UserService").Err(resultErr).Msg("Register failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "UserService").Msg("Register completed")
	}()
	firstname, lastname := strings.TrimSpace(input.Firstname), strings.TrimSpace(input.Lastname)
	login := strings.ToLower(strings.TrimSpace(input.Login))
	email, err := normalizeEmail(input.Email)
	if err != nil || firstname == "" || lastname == "" || !validLogin(login) || !validPassword(input.Password) {
		return RegisteredUser{}, ErrInvalidInput
	}

	hash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return RegisteredUser{}, err
	}

	now := s.clock.Now().UTC()
	user := domain.User{
		ID:           s.ids.NewUserID(),
		PlatformRole: domain.PlatformRoleUser,
		Firstname:    firstname,
		Lastname:     lastname,
		Email:        email,
		Login:        login,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if creator, ok := s.users.(interface {
		CreateSaved(context.Context, domain.User) (domain.User, error)
	}); ok {
		user, err = creator.CreateSaved(ctx, user)
		if err != nil {
			return RegisteredUser{}, err
		}
	} else if err := s.users.Create(ctx, user); err != nil {
		return RegisteredUser{}, err
	}
	logger := log.Ctx(ctx).With().Str("username", user.Login).Logger()
	ctx = logger.WithContext(ctx)

	return RegisteredUser{
		ID: user.ID, Email: user.Email, Login: user.Login, PlatformRole: string(user.PlatformRole),
		Firstname: user.Firstname, Lastname: user.Lastname,
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}, nil
}

func (s *UserService) Login(ctx context.Context, input LoginInput) (_ AccessToken, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "UserService").Err(resultErr).Msg("Login failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "UserService").Msg("Login completed")
	}()
	login := strings.ToLower(strings.TrimSpace(input.Login))
	if !validLogin(login) {
		return AccessToken{}, ErrInvalidCredentials
	}

	user, err := s.users.FindByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			dummy := dummyPasswordHash
			if source, ok := s.hasher.(interface{ DummyHash() string }); ok {
				dummy = source.DummyHash()
			}
			_ = s.hasher.Compare(dummy, input.Password)

			return AccessToken{}, ErrInvalidCredentials
		}

		return AccessToken{}, err
	}

	if err := s.hasher.Compare(user.PasswordHash, input.Password); err != nil {
		return AccessToken{}, ErrInvalidCredentials
	}
	logger := log.Ctx(ctx).With().Str("username", user.Login).Logger()
	ctx = logger.WithContext(ctx)

	now := s.clock.Now().UTC()
	state := domain.AuthState{UserID: user.ID, PlatformRole: user.PlatformRole}
	if source, ok := s.users.(interface {
		GetAuthState(context.Context, domain.UserID) (domain.AuthState, error)
	}); ok {
		state, err = source.GetAuthState(ctx, user.ID)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to load authentication state during login")
			return AccessToken{}, ErrInvalidCredentials
		}
	}
	if rehasher, ok := s.hasher.(interface{ NeedsRehash(string) bool }); ok && rehasher.NeedsRehash(user.PasswordHash) {
		freshHash, hashErr := s.hasher.Hash(input.Password)
		if hashErr != nil {
			return AccessToken{}, hashErr
		}
		if updater, ok := s.users.(interface {
			UpdatePasswordHashIfCurrent(context.Context, domain.UserID, string, string, time.Time) (domain.AuthState, bool, error)
		}); ok {
			updatedState, changed, updateErr := updater.UpdatePasswordHashIfCurrent(ctx, user.ID, user.PasswordHash, freshHash, now)
			if updateErr != nil {
				return AccessToken{}, updateErr
			}
			if !changed {
				return AccessToken{}, ErrInvalidCredentials
			}
			state = updatedState
		}
	}
	expiresAt := now.Add(s.accessTTL)
	value, err := s.tokens.CreateAccessToken(Session{UserID: user.ID, Username: user.Login, PlatformRole: state.PlatformRole, AuthVersion: state.AuthVersion, AuthTime: now}, nil, expiresAt)
	if err != nil {
		return AccessToken{}, err
	}

	return AccessToken{Value: value, ExpiresAt: expiresAt}, nil
}

func (s *UserService) Verify(ctx context.Context, rawToken string) (_ Principal, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "UserService").Err(resultErr).Msg("Verify failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "UserService").Msg("Verify completed")
	}()
	if strings.TrimSpace(rawToken) == "" {
		return Principal{}, ErrInvalidToken
	}

	principal, err := s.tokens.ParseAccessToken(rawToken)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	if source, ok := s.users.(interface {
		GetAuthState(context.Context, domain.UserID) (domain.AuthState, error)
	}); ok {
		state, stateErr := source.GetAuthState(ctx, principal.UserID)
		if stateErr != nil {
			log.Ctx(ctx).Error().Err(stateErr).Msg("failed to load authentication state during verification")
		}
		if stateErr != nil || state.AuthVersion != principal.AuthVersion || state.PlatformRole != principal.PlatformRole {
			return Principal{}, ErrInvalidToken
		}
	}
	return principal, nil
}

func (s *UserService) PublicJWKSet() (authjwt.JWKSet, error) {
	provider, ok := s.tokens.(interface {
		PublicJWKSet() (authjwt.JWKSet, error)
	})
	if !ok {
		return authjwt.JWKSet{}, errors.New("token manager does not expose JWKS")
	}
	return provider.PublicJWKSet()
}

func (s *UserService) UpdateUser(ctx context.Context, id domain.UserID, input UpdateUserInput) (_ RegisteredUser, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "UserService").Err(resultErr).Msg("UpdateUser failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "UserService").Msg("UpdateUser completed")
	}()
	if _, ok := s.users.(interface {
		ActorRole(context.Context, domain.UserID) (string, error)
	}); ok {
		if err := s.AuthorizeUser(ctx, id); err != nil {
			return RegisteredUser{}, err
		}
	}
	if input.Email == nil && input.Password == nil && input.Firstname == nil && input.Lastname == nil {
		return RegisteredUser{}, ErrInvalidInput
	}

	if input.Firstname != nil {
		firstname := strings.TrimSpace(*input.Firstname)
		if firstname == "" {
			return RegisteredUser{}, ErrInvalidInput
		}
		input.Firstname = &firstname
	}
	if input.Lastname != nil {
		lastname := strings.TrimSpace(*input.Lastname)
		if lastname == "" {
			return RegisteredUser{}, ErrInvalidInput
		}
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
		ID: user.ID, Login: user.Login, Email: user.Email, PlatformRole: string(user.PlatformRole),
		Firstname: user.Firstname, Lastname: user.Lastname,
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}, nil
}

func (s *UserService) DeleteUser(ctx context.Context, id domain.UserID) (resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "UserService").Err(resultErr).Msg("DeleteUser failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "UserService").Msg("DeleteUser completed")
	}()
	if _, ok := s.users.(interface {
		ActorRole(context.Context, domain.UserID) (string, error)
	}); ok {
		if err := s.AuthorizeUser(ctx, id); err != nil {
			return err
		}
	}
	if err := s.users.Delete(ctx, id); err != nil {
		return err
	}

	return nil
}

type ListUsersInput struct {
	Offset int
	Limit  int
}

type UsersPage struct {
	Users         []RegisteredUser
	NextPageToken *int
}

func (s *UserService) ListUsers(ctx context.Context, input ListUsersInput) (_ UsersPage, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "UserService").Err(resultErr).Msg("ListUsers failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "UserService").Msg("ListUsers completed")
	}()
	limit, err := normalizePageLimit(input.Offset, input.Limit)
	if err != nil {
		return UsersPage{}, err
	}
	input.Limit = limit
	// One extra row tells us whether another page exists without a count query.
	var users []domain.User
	if scoped, ok := s.users.(interface {
		ListVisible(context.Context, domain.UserID, int, int) ([]domain.User, error)
	}); ok {
		principal, authenticated := PrincipalFromContext(ctx)
		if !authenticated {
			return UsersPage{}, ErrInvalidToken
		}
		users, err = scoped.ListVisible(ctx, principal.UserID, input.Offset, input.Limit+1)
	} else {
		if _, authenticated := PrincipalFromContext(ctx); authenticated {
			return UsersPage{}, errors.New("repository does not support scoped user listing")
		}
		users, err = s.users.List(ctx, input.Offset, input.Limit+1)
	}
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
		result = append(result, RegisteredUser{ID: user.ID, Login: user.Login, Email: user.Email, PlatformRole: string(user.PlatformRole), Firstname: user.Firstname, Lastname: user.Lastname})
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
	return utf8.RuneCountInString(password) >= minimumPasswordLength && utf8.RuneCountInString(password) <= maximumBcryptLength
}

var loginPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,31}$`)

func validLogin(login string) bool { return loginPattern.MatchString(login) }

// This is a bcrypt hash of an arbitrary value. It is used only to reduce
// account-enumeration timing differences when the account does not exist.
const dummyPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (s *UserService) GetUser(ctx context.Context, target domain.UserID) (_ RegisteredUser, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "UserService").Err(resultErr).Msg("GetUser failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "UserService").Msg("GetUser completed")
	}()
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		return RegisteredUser{}, ErrInvalidToken
	}
	scoped, ok := s.users.(interface {
		GetVisible(context.Context, domain.UserID, domain.UserID) (domain.User, error)
	})
	if !ok {
		return RegisteredUser{}, domain.ErrUserNotFound
	}
	u, err := scoped.GetVisible(ctx, principal.UserID, target)
	if err != nil {
		return RegisteredUser{}, err
	}
	return RegisteredUser{ID: u.ID, Login: u.Login, Email: u.Email, PlatformRole: string(u.PlatformRole), Firstname: u.Firstname, Lastname: u.Lastname}, nil
}

func (s *UserService) GetUserByLogin(ctx context.Context, login string) (_ RegisteredUser, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "UserService").Err(resultErr).Msg("GetUserByLogin failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "UserService").Msg("GetUserByLogin completed")
	}()
	if _, ok := PrincipalFromContext(ctx); !ok || !validLogin(strings.ToLower(strings.TrimSpace(login))) {
		return RegisteredUser{}, ErrInvalidToken
	}
	u, err := s.users.FindByLogin(ctx, strings.ToLower(strings.TrimSpace(login)))
	if err != nil {
		return RegisteredUser{}, err
	}
	return RegisteredUser{ID: u.ID, Login: u.Login, Email: u.Email, PlatformRole: string(u.PlatformRole), Firstname: u.Firstname, Lastname: u.Lastname}, nil
}
func (s *UserService) AuthorizeUser(ctx context.Context, target domain.UserID) (resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "UserService").Err(resultErr).Msg("AuthorizeUser failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "UserService").Msg("AuthorizeUser completed")
	}()
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		return ErrInvalidToken
	}
	if principal.UserID == target {
		return nil
	}
	repo, ok := s.users.(interface {
		ActorRole(context.Context, domain.UserID) (string, error)
	})
	if !ok {
		return ErrForbidden
	}
	role, err := repo.ActorRole(ctx, principal.UserID)
	if err != nil {
		return err
	}
	if role != "admin" {
		return ErrForbidden
	}
	return nil
}
