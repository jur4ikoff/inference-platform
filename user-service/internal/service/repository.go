package service

import (
	"context"
	"time"

	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/utils/authjwt"
)

// UserRepository is the persistence capability required by UserService.
// PostgreSQL is only one possible implementation of this interface.
type UserRepository interface {
	Create(ctx context.Context, user domain.User) error
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	FindByLogin(ctx context.Context, login string) (domain.User, error)
	List(ctx context.Context, offset, limit int) ([]domain.User, error)
	Update(ctx context.Context, id domain.UserID, changes domain.UserUpdate, updatedAt time.Time) (domain.User, error)
	Delete(ctx context.Context, id domain.UserID) error
}

// ProjectRepository describes the project persistence operations required by
// ProjectService.
type ProjectRepository interface {
	Create(context.Context, domain.UserID, string, string) (domain.Project, error)
	List(context.Context, domain.UserID, string, int, int) ([]domain.Project, error)
	Get(context.Context, domain.UserID, string) (domain.Project, error)
	Update(context.Context, domain.UserID, string, string) (domain.Project, error)
	Delete(context.Context, domain.UserID, string) error
	Members(context.Context, domain.UserID, string, int, int) ([]domain.Member, error)
	ChangeMember(context.Context, domain.UserID, string, domain.UserID, domain.RoleID, bool) (bool, error)
}

type RoleRepository interface {
	Create(context.Context, domain.UserID, domain.Role) (domain.Role, error)
	Get(context.Context, domain.RoleID) (domain.Role, error)
	List(context.Context, string, int, int) ([]domain.Role, error)
	Update(context.Context, domain.UserID, domain.RoleID, domain.RoleUpdate) (domain.Role, error)
	Delete(context.Context, domain.UserID, domain.RoleID) error
}

type PlatformRoleRepository interface {
	SetPlatformRole(context.Context, domain.UserID, domain.UserID, domain.PlatformRole) (domain.User, error)
}

type ProjectTokenRepository interface {
	ProjectTokenGrant(context.Context, domain.UserID, string) (authjwt.ProjectGrant, error)
}

type BootstrapAdminRepository interface {
	BootstrapAdmin(context.Context, domain.User) (domain.User, bool, error)
}
