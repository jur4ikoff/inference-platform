package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/utils/authjwt"
	"github.com/rs/zerolog/log"
)

type authService interface {
	Register(context.Context, service.RegisterInput) (service.RegisteredUser, error)
	Login(context.Context, service.LoginInput) (service.AccessToken, error)
	Verify(context.Context, string) (service.Principal, error)
	PublicJWKSet() (authjwt.JWKSet, error)
}

// AuthHandler adapts authentication-related HTTP requests to UserService.
type AuthHandler struct {
	authService   authService
	projectTokens projectTokenService
}

type projectTokenService interface {
	Create(context.Context, string) (service.AccessToken, error)
}

func NewAuthHandler(authService authService, projectTokens ...projectTokenService) *AuthHandler {
	h := &AuthHandler{authService: authService}
	if len(projectTokens) > 0 {
		h.projectTokens = projectTokens[0]
	}
	return h
}

func (h *AuthHandler) ProjectToken(ctx context.Context, input *ProjectTokenInput) (*ProjectTokenOutput, error) {
	if h.projectTokens == nil {
		return nil, huma.Error404NotFound("resource not found")
	}
	token, err := h.projectTokens.Create(ctx, input.Body.ProjectName)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &ProjectTokenOutput{Body: AccessTokenResponse{AccessToken: token.Value, TokenType: "Bearer", ExpiresAt: token.ExpiresAt}}, nil
}

func (h *AuthHandler) JWKS(w http.ResponseWriter, r *http.Request) {
	keys, err := h.authService.PublicJWKSet()
	if err != nil {
		log.Ctx(r.Context()).Error().Err(err).Msg("failed to load public keys")
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	log.Ctx(r.Context()).Info().Msg("public keys loaded")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_ = json.NewEncoder(w).Encode(keys)
}

func (h *AuthHandler) Middleware(api huma.API) AuthMiddleware {
	return AuthenticationMiddleware(api, h.authService)
}

func (h *AuthHandler) Register(ctx context.Context, input *RegisterInput) (*RegisterOutput, error) {
	user, err := h.authService.Register(ctx, service.RegisterInput{
		Login:     input.Body.Login,
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
			Login:        user.Login,
			ID:           string(user.ID),
			PlatformRole: user.PlatformRole,
			Email:        user.Email,
			Firstname:    user.Firstname,
			Lastname:     user.Lastname,
		},
	}, nil
}

func (h *AuthHandler) Login(ctx context.Context, input *LoginInput) (*LoginOutput, error) {
	token, err := h.authService.Login(ctx, service.LoginInput{
		Login:    input.Body.Login,
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
	case errors.Is(err, service.ErrInvalidToken):
		return huma.Error401Unauthorized("missing or invalid bearer token")
	case errors.Is(err, service.ErrForbidden):
		return huma.Error403Forbidden("access denied")
	case errors.Is(err, service.ErrInvalidInput):
		return huma.Error400BadRequest("invalid request")
	case errors.Is(err, domain.ErrLastAdministrator), errors.Is(err, domain.ErrLastTeamlead):
		return huma.Error409Conflict("operation would remove the last administrator or project teamlead")
	case errors.Is(err, domain.ErrProjectExists):
		return huma.Error409Conflict("project name is already reserved")
	case errors.Is(err, domain.ErrProjectNotFound), errors.Is(err, domain.ErrMemberNotFound), errors.Is(err, domain.ErrRoleNotFound):
		return huma.Error404NotFound("resource not found")
	case errors.Is(err, domain.ErrRoleAlreadyExists):
		return huma.Error409Conflict("role name is already registered")
	case errors.Is(err, domain.ErrRoleAssigned), errors.Is(err, domain.ErrSystemRole):
		return huma.Error409Conflict("role is protected or assigned")
	case errors.Is(err, domain.ErrEmailAlreadyTaken):
		return huma.Error409Conflict("email is already registered")
	case errors.Is(err, domain.ErrLoginAlreadyTaken):
		return huma.Error409Conflict("login is already registered")
	case errors.Is(err, service.ErrInvalidCredentials):
		return huma.Error401Unauthorized("invalid email or password")
	case errors.Is(err, domain.ErrUserNotFound):
		return huma.Error404NotFound("user not found")
	default:
		return huma.Error500InternalServerError("internal server error")
	}
}
