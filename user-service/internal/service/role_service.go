package service

import (
	"context"
	"strings"

	"github.com/jur4ikoff/user-service/internal/domain"
)

type RoleRepository interface {
	List(ctx context.Context, name string, offset, limit int) ([]domain.Role, error)
}

type RoleService struct {
	roles RoleRepository
}

func NewRoleService(roles RoleRepository) *RoleService {
	return &RoleService{roles: roles}
}

type ListRolesInput struct {
	Name   string
	Offset int
	Limit  int
}

type RolesPage struct {
	Roles         []domain.Role
	NextPageToken *int
}

func (s *RoleService) ListRoles(ctx context.Context, input ListRolesInput) (RolesPage, error) {
	limit, err := normalizePageLimit(input.Offset, input.Limit)
	if err != nil {
		return RolesPage{}, err
	}
	roles, err := s.roles.List(ctx, strings.TrimSpace(input.Name), input.Offset, limit+1)
	if err != nil {
		return RolesPage{}, err
	}
	page := RolesPage{}
	if len(roles) > limit {
		nextOffset := input.Offset + limit
		page.NextPageToken = &nextOffset
		roles = roles[:limit]
	}
	page.Roles = make([]domain.Role, len(roles))
	copy(page.Roles, roles)
	return page, nil
}
