package service

import (
	"context"
	"github.com/rs/zerolog/log"
	"strings"

	"github.com/jur4ikoff/user-service/internal/domain"
)

type RoleService struct{ roles RoleRepository }

func NewRoleService(roles RoleRepository) *RoleService { return &RoleService{roles: roles} }

type ListRolesInput struct {
	Name          string
	Offset, Limit int
}
type RolesPage struct {
	Roles         []domain.Role
	NextPageToken *int
}

func roleActor(ctx context.Context) (domain.UserID, error) {
	p, ok := PrincipalFromContext(ctx)
	if !ok {
		return "", ErrInvalidToken
	}
	return p.UserID, nil
}

func normalizeRoleName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 63 {
		return "", ErrInvalidInput
	}
	return value, nil
}

func (s *RoleService) CreateRole(ctx context.Context, role domain.Role) (_ domain.Role, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "RoleService").Err(resultErr).Msg("CreateRole failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "RoleService").Msg("CreateRole completed")
	}()
	actor, err := roleActor(ctx)
	if err != nil {
		return domain.Role{}, err
	}
	role.Name, err = normalizeRoleName(role.Name)
	if err != nil {
		return domain.Role{}, err
	}
	return s.roles.Create(ctx, actor, role)
}

func (s *RoleService) GetRole(ctx context.Context, id domain.RoleID) (_ domain.Role, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "RoleService").Err(resultErr).Msg("GetRole failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "RoleService").Msg("GetRole completed")
	}()
	if _, err := roleActor(ctx); err != nil {
		return domain.Role{}, err
	}
	if id <= 0 {
		return domain.Role{}, ErrInvalidInput
	}
	return s.roles.Get(ctx, id)
}

func (s *RoleService) ListRoles(ctx context.Context, input ListRolesInput) (_ RolesPage, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "RoleService").Err(resultErr).Msg("ListRoles failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "RoleService").Msg("ListRoles completed")
	}()
	if _, err := roleActor(ctx); err != nil {
		return RolesPage{}, err
	}
	limit, err := normalizePageLimit(input.Offset, input.Limit)
	if err != nil {
		return RolesPage{}, err
	}
	roles, err := s.roles.List(ctx, strings.TrimSpace(input.Name), input.Offset, limit+1)
	if err != nil {
		return RolesPage{}, err
	}
	page := RolesPage{Roles: roles}
	if len(roles) > limit {
		next := input.Offset + limit
		page.NextPageToken = &next
		page.Roles = roles[:limit]
	}
	return page, nil
}

func (s *RoleService) UpdateRole(ctx context.Context, id domain.RoleID, update domain.RoleUpdate) (_ domain.Role, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "RoleService").Err(resultErr).Msg("UpdateRole failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "RoleService").Msg("UpdateRole completed")
	}()
	actor, err := roleActor(ctx)
	if err != nil {
		return domain.Role{}, err
	}
	if id <= 0 {
		return domain.Role{}, ErrInvalidInput
	}
	if update.Name == nil && update.AllPermissions == nil && update.CanReadProject == nil && update.CanUpdateProject == nil && update.CanDeleteProject == nil && update.CanManageMembers == nil && update.CanCreateModel == nil && update.CanReadModel == nil && update.CanUpdateModel == nil && update.CanDeleteModel == nil && update.CanStartModel == nil && update.CanStopModel == nil {
		return domain.Role{}, ErrInvalidInput
	}
	if update.Name != nil {
		name, err := normalizeRoleName(*update.Name)
		if err != nil {
			return domain.Role{}, err
		}
		update.Name = &name
	}
	return s.roles.Update(ctx, actor, id, update)
}

func (s *RoleService) DeleteRole(ctx context.Context, id domain.RoleID) (resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "RoleService").Err(resultErr).Msg("DeleteRole failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "RoleService").Msg("DeleteRole completed")
	}()
	actor, err := roleActor(ctx)
	if err != nil {
		return err
	}
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.roles.Delete(ctx, actor, id)
}

type PlatformRoleService struct{ users PlatformRoleRepository }

func NewPlatformRoleService(users PlatformRoleRepository) *PlatformRoleService {
	return &PlatformRoleService{users: users}
}
func (s *PlatformRoleService) SetRole(ctx context.Context, target domain.UserID, role domain.PlatformRole) (_ RegisteredUser, resultErr error) {
	defer func() {
		if resultErr != nil {
			log.Ctx(ctx).Error().Str("component", "PlatformRoleService").Err(resultErr).Msg("SetRole failed")
			return
		}
		log.Ctx(ctx).Info().Str("component", "PlatformRoleService").Msg("SetRole completed")
	}()
	actor, err := roleActor(ctx)
	if err != nil {
		return RegisteredUser{}, err
	}
	if _, err = domain.ParseUserID(string(target)); err != nil {
		return RegisteredUser{}, ErrInvalidInput
	}
	if err = role.Validate(); err != nil {
		return RegisteredUser{}, ErrInvalidInput
	}
	u, err := s.users.SetPlatformRole(ctx, actor, target, role)
	if err != nil {
		return RegisteredUser{}, err
	}
	return RegisteredUser{ID: u.ID, Email: u.Email, PlatformRole: string(u.PlatformRole), Firstname: u.Firstname, Lastname: u.Lastname, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}, nil
}
