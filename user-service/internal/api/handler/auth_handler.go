package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

type authService interface {
	Register(context.Context, service.RegisterInput) (service.RegisteredUser, error)
	Login(context.Context, service.LoginInput) (service.AccessToken, error)
	Verify(context.Context, string) (service.Principal, error)
}

// AuthHandler adapts authentication-related HTTP requests to UserService.
type AuthHandler struct {
	authService authService
}

func NewAuthHandler(authService authService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Middleware(api huma.API) AuthMiddleware {
	return AuthenticationMiddleware(api, h.authService)
}

func (h *AuthHandler) Register(ctx context.Context, input *RegisterInput) (*RegisterOutput, error) {
	user, err := h.authService.Register(ctx, service.RegisterInput{
		Firstname: input.Body.Firstname,
		Lastname:  input.Body.Lastname,
		Email:     input.Body.Email,
		Password:  input.Body.Password,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}

	return &RegisterOutput{
		Body: RegisteredUserResponse{
			ID:        string(user.ID),
			Email:     user.Email,
			Firstname: user.Firstname,
			Lastname:  user.Lastname,
		},
	}, nil
}

func (h *AuthHandler) Login(ctx context.Context, input *LoginInput) (*LoginOutput, error) {
	token, err := h.authService.Login(ctx, service.LoginInput{
		Email:    input.Body.Email,
		Password: input.Body.Password,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}

	return &LoginOutput{
		Body: AccessTokenResponse{
			AccessToken: token.Value,
			TokenType:   "Bearer",
			ExpiresAt:   token.ExpiresAt,
		},
	}, nil
}

// Authenticate handles internal auth_request subrequests from the reverse proxy.
func (h *AuthHandler) Authenticate(w http.ResponseWriter, r *http.Request) {
	rawToken, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
		return
	}
	principal, err := h.authService.Verify(r.Context(), rawToken)
	if err != nil {
		http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
		return
	}
	w.Header().Set("X-User-ID", string(principal.UserID))
	w.WriteHeader(http.StatusNoContent)
}

func mapServiceError(err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return huma.Error400BadRequest("invalid request")
	case errors.Is(err, domain.ErrEmailAlreadyTaken):
		return huma.Error409Conflict("email is already registered")
	case errors.Is(err, service.ErrInvalidCredentials):
		return huma.Error401Unauthorized("invalid email or password")
	case errors.Is(err, domain.ErrUserNotFound):
		return huma.Error404NotFound("user not found")
	default:
		return huma.Error500InternalServerError("internal server error")
	}
}
