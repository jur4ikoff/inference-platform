package handler

import (
	"context"

	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

type roleService interface {
	CreateRole(context.Context, domain.Role) (domain.Role, error)
	GetRole(context.Context, domain.RoleID) (domain.Role, error)
	ListRoles(context.Context, service.ListRolesInput) (service.RolesPage, error)
	UpdateRole(context.Context, domain.RoleID, domain.RoleUpdate) (domain.Role, error)
	DeleteRole(context.Context, domain.RoleID) error
}

type RoleHandler struct{ roles roleService }

func NewRoleHandler(roles roleService) *RoleHandler { return &RoleHandler{roles: roles} }

func (h *RoleHandler) CreateRole(ctx context.Context, in *CreateRoleInput) (*RoleOutput, error) {
	r, err := h.roles.CreateRole(ctx, toRole(in.Body))
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &RoleOutput{Body: roleResponse(r)}, nil
}
func (h *RoleHandler) GetRole(ctx context.Context, in *GetRoleInput) (*RoleOutput, error) {
	r, err := h.roles.GetRole(ctx, domain.RoleID(in.ID))
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &RoleOutput{Body: roleResponse(r)}, nil
}
func (h *RoleHandler) ListRoles(ctx context.Context, in *ListRolesInput) (*ListRolesOutput, error) {
	page, err := h.roles.ListRoles(ctx, service.ListRolesInput{Name: in.Name, Offset: in.Offset, Limit: in.Limit})
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := make([]RoleResponse, 0, len(page.Roles))
	for _, role := range page.Roles {
		out = append(out, roleResponse(role))
	}
	return &ListRolesOutput{Body: ListRolesResponse{Roles: out, NextPageToken: page.NextPageToken}}, nil
}
func (h *RoleHandler) UpdateRole(ctx context.Context, in *UpdateRoleInput) (*RoleOutput, error) {
	r, err := h.roles.UpdateRole(ctx, domain.RoleID(in.ID), toRoleUpdate(in.Body))
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &RoleOutput{Body: roleResponse(r)}, nil
}
func (h *RoleHandler) DeleteRole(ctx context.Context, in *DeleteRoleInput) (*DeleteRoleOutput, error) {
	if err := h.roles.DeleteRole(ctx, domain.RoleID(in.ID)); err != nil {
		return nil, mapServiceError(err)
	}
	return &DeleteRoleOutput{}, nil
}
func toRole(in CreateRoleRequest) domain.Role {
	return domain.Role{Name: in.Name, AllPermissions: in.AllPermissions, CanReadProject: in.CanReadProject, CanUpdateProject: in.CanUpdateProject, CanDeleteProject: in.CanDeleteProject, CanManageMembers: in.CanManageMembers, CanCreateModel: in.CanCreateModel, CanReadModel: in.CanReadModel, CanUpdateModel: in.CanUpdateModel, CanDeleteModel: in.CanDeleteModel, CanStartModel: in.CanStartModel, CanStopModel: in.CanStopModel}
}
func toRoleUpdate(in UpdateRoleRequest) domain.RoleUpdate {
	return domain.RoleUpdate{Name: in.Name, AllPermissions: in.AllPermissions, CanReadProject: in.CanReadProject, CanUpdateProject: in.CanUpdateProject, CanDeleteProject: in.CanDeleteProject, CanManageMembers: in.CanManageMembers, CanCreateModel: in.CanCreateModel, CanReadModel: in.CanReadModel, CanUpdateModel: in.CanUpdateModel, CanDeleteModel: in.CanDeleteModel, CanStartModel: in.CanStartModel, CanStopModel: in.CanStopModel}
}
func roleResponse(role domain.Role) RoleResponse {
	return RoleResponse{ID: int(role.ID), Name: role.Name, AllPermissions: role.AllPermissions, CanReadProject: role.CanReadProject, CanUpdateProject: role.CanUpdateProject, CanDeleteProject: role.CanDeleteProject, CanManageMembers: role.CanManageMembers, CanCreateModel: role.CanCreateModel, CanReadModel: role.CanReadModel, CanUpdateModel: role.CanUpdateModel, CanDeleteModel: role.CanDeleteModel, CanStartModel: role.CanStartModel, CanStopModel: role.CanStopModel}
}
