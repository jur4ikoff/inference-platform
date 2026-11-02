package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/user-service/internal/domain"
)

type RoleRepository struct {
	pool *pgxpool.Pool
}

func NewRoleRepository(pool *pgxpool.Pool) *RoleRepository {
	return &RoleRepository{pool: pool}
}

func (r *RoleRepository) List(ctx context.Context, name string, offset, limit int) ([]domain.Role, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, all_permissions,
			can_read_project, can_update_project, can_delete_project, can_manage_members,
			can_create_model, can_read_model, can_update_model, can_delete_model,
			can_start_model, can_stop_model
		FROM roles
		WHERE ($1 = '' OR name = $1)
		ORDER BY created_at, id
		LIMIT $2 OFFSET $3`, name, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := make([]domain.Role, 0)
	for rows.Next() {
		var role domain.Role
		if err := rows.Scan(
			&role.ID, &role.Name, &role.AllPermissions,
			&role.CanReadProject, &role.CanUpdateProject, &role.CanDeleteProject, &role.CanManageMembers,
			&role.CanCreateModel, &role.CanReadModel, &role.CanUpdateModel, &role.CanDeleteModel,
			&role.CanStartModel, &role.CanStopModel,
		); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}
