package service

import (
	"context"
	"github.com/rs/zerolog/log"
	"strings"

	"github.com/jur4ikoff/user-service/internal/domain"
)

type BootstrapAdminService struct {
	repository BootstrapAdminRepository
	hasher     PasswordHasher
	ids        IDGenerator
	clock      Clock
}

func NewBootstrapAdminService(repository BootstrapAdminRepository, hasher PasswordHasher, ids IDGenerator, clock Clock) *BootstrapAdminService {
	return &BootstrapAdminService{repository: repository, hasher: hasher, ids: ids, clock: clock}
}

// Create creates the sole first administrator or returns the existing account
// unchanged when the same admin is bootstrapped again.
func (s *BootstrapAdminService) Create(ctx context.Context, input RegisterInput) (_ RegisteredUser, _ bool, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "BootstrapAdminService").Err(resultErr).Msg("Create failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "BootstrapAdminService").Msg("Create completed")
	}()
	email, err := normalizeEmail(input.Email)
	login := strings.ToLower(strings.TrimSpace(input.Login))
	firstname, lastname := strings.TrimSpace(input.Firstname), strings.TrimSpace(input.Lastname)
	if err != nil || firstname == "" || lastname == "" || !validLogin(login) || !validPassword(input.Password) {
		return RegisteredUser{}, false, ErrInvalidInput
	}
	hash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return RegisteredUser{}, false, err
	}
	now := s.clock.Now().UTC()
	user, existed, err := s.repository.BootstrapAdmin(ctx, domain.User{ID: s.ids.NewUserID(), Login: login, Email: email, Firstname: firstname, Lastname: lastname, PasswordHash: hash, PlatformRole: domain.PlatformRoleAdmin, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return RegisteredUser{}, false, err
	}
	return RegisteredUser{ID: user.ID, Email: user.Email, PlatformRole: string(user.PlatformRole), Firstname: user.Firstname, Lastname: user.Lastname, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}, existed, nil
}
