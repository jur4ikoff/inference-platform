package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/user-service/internal/domain"
)

type ProjectRepository struct {
	pool *pgxpool.Pool
}

func NewProjectRepository(pool *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{pool: pool}
}

func (r *ProjectRepository) AddUserToProject(ctx context.Context, projectID domain.ProjectID, userID domain.UserID, roleID domain.RoleID) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO members (id, created_at, updated_at, user_id, project_id, role_id)
		VALUES ($1, $2, $2, $3, $4, $5)`, uuid.NewString(), now, userID, projectID, roleID)
	return err
}

func (r *ProjectRepository) List(ctx context.Context, userID domain.UserID, name string, offset, limit int) ([]domain.Project, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.name, p.description
		FROM projects p
		JOIN members m ON m.project_id = p.id
		WHERE m.user_id = $1 AND ($2 = '' OR p.name = $2)
		ORDER BY p.created_at, p.id
		LIMIT $3 OFFSET $4`, userID, name, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := make([]domain.Project, 0)
	for rows.Next() {
		var project domain.Project
		if err := rows.Scan(&project.ID, &project.Name, &project.Description); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}
