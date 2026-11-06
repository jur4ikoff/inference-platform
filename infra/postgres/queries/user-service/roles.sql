-- name: CreateRole :one
INSERT INTO roles (name, created_at, updated_at, all_permissions, can_read_project, can_update_project, can_delete_project, can_manage_members, can_create_model, can_read_model, can_update_model, can_delete_model, can_start_model, can_stop_model)
VALUES ($1,$2,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id, name, system_key, all_permissions, can_read_project, can_update_project, can_delete_project, can_manage_members, can_create_model, can_read_model, can_update_model, can_delete_model, can_start_model, can_stop_model;

-- name: GetRole :one
SELECT id, name, system_key, all_permissions, can_read_project, can_update_project, can_delete_project, can_manage_members, can_create_model, can_read_model, can_update_model, can_delete_model, can_start_model, can_stop_model FROM roles WHERE id = $1;

-- name: ListRoles :many
SELECT id, name, system_key, all_permissions, can_read_project, can_update_project, can_delete_project, can_manage_members, can_create_model, can_read_model, can_update_model, can_delete_model, can_start_model, can_stop_model FROM roles WHERE (sqlc.arg(name)::text = '' OR name = sqlc.arg(name)) ORDER BY created_at, id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: LockRole :one
SELECT id, name, system_key, all_permissions, can_read_project, can_update_project, can_delete_project, can_manage_members, can_create_model, can_read_model, can_update_model, can_delete_model, can_start_model, can_stop_model FROM roles WHERE id = $1 FOR UPDATE;

-- name: UpdateRole :one
UPDATE roles SET name = COALESCE(sqlc.narg(name), name), all_permissions = COALESCE(sqlc.narg(all_permissions), all_permissions), can_read_project = COALESCE(sqlc.narg(can_read_project), can_read_project), can_update_project = COALESCE(sqlc.narg(can_update_project), can_update_project), can_delete_project = COALESCE(sqlc.narg(can_delete_project), can_delete_project), can_manage_members = COALESCE(sqlc.narg(can_manage_members), can_manage_members), can_create_model = COALESCE(sqlc.narg(can_create_model), can_create_model), can_read_model = COALESCE(sqlc.narg(can_read_model), can_read_model), can_update_model = COALESCE(sqlc.narg(can_update_model), can_update_model), can_delete_model = COALESCE(sqlc.narg(can_delete_model), can_delete_model), can_start_model = COALESCE(sqlc.narg(can_start_model), can_start_model), can_stop_model = COALESCE(sqlc.narg(can_stop_model), can_stop_model), updated_at = $2 WHERE id = $1
RETURNING id, name, system_key, all_permissions, can_read_project, can_update_project, can_delete_project, can_manage_members, can_create_model, can_read_model, can_update_model, can_delete_model, can_start_model, can_stop_model;

-- name: DeleteRole :execrows
DELETE FROM roles WHERE id = $1;

-- name: AuditRoleChange :exec
INSERT INTO audit_events (id, actor_id, action, object_type, object_id, created_at, details) VALUES ($1, $2, $3, 'role', $4, $5, $6);
