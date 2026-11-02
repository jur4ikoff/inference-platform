package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

type roleLister interface {
	ListRoles(context.Context, service.ListRolesInput) (service.RolesPage, error)
}

type RoleHandler struct {
	roles roleLister
}

func NewRoleHandler(roles roleLister) *RoleHandler {
	return &RoleHandler{roles: roles}
}

func (h *RoleHandler) CreateRole(context.Context, *CreateRoleInput) (*RoleOutput, error) {
	return nil, huma.Error501NotImplemented("create role endpoint is not implemented")
}

func (h *RoleHandler) GetRole(context.Context, *GetRoleInput) (*RoleOutput, error) {
	return nil, huma.Error501NotImplemented("get role endpoint is not implemented")
}

func (h *RoleHandler) ListRoles(ctx context.Context, input *ListRolesInput) (*ListRolesOutput, error) {
	if _, ok := PrincipalFromContext(ctx); !ok {
		return nil, huma.Error401Unauthorized("missing or invalid bearer token")
	}
	page, err := h.roles.ListRoles(ctx, service.ListRolesInput{Name: input.Name, Offset: input.Offset, Limit: input.Limit})
	if err != nil {
		return nil, mapServiceError(err)
	}
	roles := make([]RoleResponse, 0, len(page.Roles))
	for _, role := range page.Roles {
		roles = append(roles, roleResponse(role))
	}
	return &ListRolesOutput{Body: ListRolesResponse{Roles: roles, NextPageToken: page.NextPageToken}}, nil
}

func (h *RoleHandler) UpdateRole(context.Context, *UpdateRoleInput) (*RoleOutput, error) {
	return nil, huma.Error501NotImplemented("update role endpoint is not implemented")
}

func (h *RoleHandler) DeleteRole(context.Context, *DeleteRoleInput) (*DeleteRoleOutput, error) {
	return nil, huma.Error501NotImplemented("delete role endpoint is not implemented")
}

func roleResponse(role domain.Role) RoleResponse {
	return RoleResponse{
		ID:               int(role.ID),
		Name:             role.Name,
		AllPermissions:   role.AllPermissions,
		CanReadProject:   role.CanReadProject,
		CanUpdateProject: role.CanUpdateProject,
		CanDeleteProject: role.CanDeleteProject,
		CanManageMembers: role.CanManageMembers,
		CanCreateModel:   role.CanCreateModel,
		CanReadModel:     role.CanReadModel,
		CanUpdateModel:   role.CanUpdateModel,
		CanDeleteModel:   role.CanDeleteModel,
		CanStartModel:    role.CanStartModel,
		CanStopModel:     role.CanStopModel,
	}
}
