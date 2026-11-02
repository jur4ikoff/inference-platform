package service

import (
	"context"
	"time"

	"github.com/jur4ikoff/user-service/internal/domain"
)

// UserRepository is the persistence capability required by UserService.
// PostgreSQL is only one possible implementation of this interface.
type UserRepository interface {
	Create(ctx context.Context, user domain.User) error
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	List(ctx context.Context, offset, limit int) ([]domain.User, error)
	Update(ctx context.Context, id domain.UserID, changes domain.UserUpdate, updatedAt time.Time) (domain.User, error)
	Delete(ctx context.Context, id domain.UserID) error
}

// ProjectRepository describes the project persistence operations required by
// ProjectService.
type ProjectRepository interface {
	AddUserToProject(ctx context.Context, projectID domain.ProjectID, userID domain.UserID, roleID domain.RoleID) error
	List(ctx context.Context, userID domain.UserID, name string, offset, limit int) ([]domain.Project, error)
}
