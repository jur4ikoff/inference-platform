package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

type userService interface {
	ListUsers(context.Context, service.ListUsersInput) (service.UsersPage, error)
	UpdateUser(context.Context, domain.UserID, service.UpdateUserInput) (service.RegisteredUser, error)
	DeleteUser(context.Context, domain.UserID) error
}

// UserHandler adapts user-management HTTP requests to UserService.
type UserHandler struct {
	userService userService
}

func NewUserHandler(userService userService) *UserHandler {
	return &UserHandler{userService: userService}
}

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
			ID: string(user.ID), Email: user.Email, Firstname: user.Firstname, Lastname: user.Lastname,
		})
	}
	return &ListUsersOutput{Body: ListUsersResponse{Users: response, NextPageToken: page.NextPageToken}}, nil
}

func (h *UserHandler) UpdateUser(ctx context.Context, input *UpdateUserInput) (*UpdateUserOutput, error) {
	userID, err := authorizeUser(ctx, input.ID)
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
		RegisteredUserResponse: RegisteredUserResponse{ID: string(user.ID), Email: user.Email, Firstname: user.Firstname, Lastname: user.Lastname},
		CreatedAt:              user.CreatedAt,
		UpdatedAt:              user.UpdatedAt,
	}}, nil
}

func (h *UserHandler) DeleteUser(ctx context.Context, input *DeleteUserInput) (*DeleteUserOutput, error) {
	userID, err := authorizeUser(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if err := h.userService.DeleteUser(ctx, userID); err != nil {
		return nil, mapServiceError(err)
	}
	return &DeleteUserOutput{}, nil
}

func (h *UserHandler) GetUserByID(context.Context, *GetUserInput) (*GetUserOutput, error) {
	return nil, huma.Error501NotImplemented("get user endpoint is not implemented")
}

func authorizeUser(ctx context.Context, requestedID string) (domain.UserID, error) {
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		return "", huma.Error401Unauthorized("missing or invalid bearer token")
	}
	userID := domain.UserID(requestedID)
	if principal.UserID != userID {
		return "", huma.Error403Forbidden("you can only modify your own user")
	}
	return userID, nil
}
