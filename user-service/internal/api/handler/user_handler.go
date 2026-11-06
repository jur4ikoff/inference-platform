package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

type userService interface {
	GetUser(context.Context, domain.UserID) (service.RegisteredUser, error)
	GetUserByLogin(context.Context, string) (service.RegisteredUser, error)
	ListUsers(context.Context, service.ListUsersInput) (service.UsersPage, error)
	UpdateUser(context.Context, domain.UserID, service.UpdateUserInput) (service.RegisteredUser, error)
	DeleteUser(context.Context, domain.UserID) error
	AuthorizeUser(context.Context, domain.UserID) error
}
type platformRoleService interface {
	SetRole(context.Context, domain.UserID, domain.PlatformRole) (service.RegisteredUser, error)
}

// UserHandler adapts user-management HTTP requests to UserService.
type UserHandler struct {
	userService   userService
	platformRoles platformRoleService
}

func NewUserHandler(userService userService, platformRoles ...platformRoleService) *UserHandler {
	h := &UserHandler{userService: userService}
	if len(platformRoles) > 0 {
		h.platformRoles = platformRoles[0]
	}
	return h
}

func (h *UserHandler) HasPlatformRoleService() bool { return h.platformRoles != nil }

func (h *UserHandler) ListUsers(ctx context.Context, input *ListUsersInput) (*ListUsersOutput, error) {
	_, ok := PrincipalFromContext(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("missing or invalid bearer token")
	}

	query := service.ListUsersInput{Offset: input.Offset, Limit: input.Limit}
	page, err := h.userService.ListUsers(ctx, query)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := make([]RegisteredUserResponse, 0, len(page.Users))
	for _, user := range page.Users {
		response = append(response, RegisteredUserResponse{
			ID: string(user.ID), Email: user.Email, PlatformRole: user.PlatformRole, Firstname: user.Firstname, Lastname: user.Lastname,
		})
	}
	return &ListUsersOutput{Body: ListUsersResponse{Users: response, NextPageToken: page.NextPageToken}}, nil
}

func (h *UserHandler) UpdateUser(ctx context.Context, input *UpdateUserInput) (*UpdateUserOutput, error) {
	userID, err := h.authorizeUser(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	user, err := h.userService.UpdateUser(ctx, userID, service.UpdateUserInput{
		Firstname: input.Body.Firstname,
		Lastname:  input.Body.Lastname,
		Email:     input.Body.Email,
		Password:  input.Body.Password,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}

	return &UpdateUserOutput{Body: UpdatedUserResponse{
		RegisteredUserResponse: RegisteredUserResponse{ID: string(user.ID), Email: user.Email, PlatformRole: user.PlatformRole, Firstname: user.Firstname, Lastname: user.Lastname},
		CreatedAt:              user.CreatedAt,
		UpdatedAt:              user.UpdatedAt,
	}}, nil
}

func (h *UserHandler) DeleteUser(ctx context.Context, input *DeleteUserInput) (*DeleteUserOutput, error) {
	userID, err := h.authorizeUser(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if err := h.userService.DeleteUser(ctx, userID); err != nil {
		return nil, mapServiceError(err)
	}
	return &DeleteUserOutput{}, nil
}

func (h *UserHandler) GetUserByID(ctx context.Context, input *GetUserInput) (*GetUserOutput, error) {
	u, err := h.userService.GetUser(ctx, domain.UserID(input.ID))
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &GetUserOutput{Body: RegisteredUserResponse{ID: string(u.ID), Email: u.Email, PlatformRole: u.PlatformRole, Firstname: u.Firstname, Lastname: u.Lastname}}, nil
}
func (h *UserHandler) GetUserByLogin(ctx context.Context, input *GetUserByLoginInput) (*GetUserOutput, error) {
	u, err := h.userService.GetUserByLogin(ctx, input.Login)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &GetUserOutput{Body: RegisteredUserResponse{ID: string(u.ID), Login: u.Login, Email: u.Email, PlatformRole: u.PlatformRole, Firstname: u.Firstname, Lastname: u.Lastname}}, nil
}

func (h *UserHandler) SetPlatformRole(ctx context.Context, input *SetPlatformRoleInput) (*UpdateUserOutput, error) {
	user, err := h.platformRoles.SetRole(ctx, domain.UserID(input.ID), domain.PlatformRole(input.Body.PlatformRole))
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &UpdateUserOutput{Body: UpdatedUserResponse{RegisteredUserResponse: RegisteredUserResponse{ID: string(user.ID), Email: user.Email, PlatformRole: user.PlatformRole, Firstname: user.Firstname, Lastname: user.Lastname}, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}}, nil
}

func (h *UserHandler) authorizeUser(ctx context.Context, requestedID string) (domain.UserID, error) {
	_, ok := PrincipalFromContext(ctx)
	if !ok {
		return "", huma.Error401Unauthorized("missing or invalid bearer token")
	}
	userID := domain.UserID(requestedID)
	if err := h.userService.AuthorizeUser(ctx, userID); err != nil {
		return "", mapServiceError(err)
	}
	return userID, nil
}
