-- name: InsertProject :one
INSERT INTO projects (id, name, description, created_at, updated_at) VALUES ($1, $2, $3, $4, $4) RETURNING id, name, description;

-- name: InsertTeamlead :execrows
INSERT INTO members (id, user_id, project_id, role_id, created_at, updated_at)
SELECT $1, $2, $3, id, $4, $4 FROM roles WHERE system_key = 'teamlead';

-- name: ListVisibleProjects :many
SELECT p.id, p.name, p.description FROM projects p
WHERE p.deleted_at IS NULL AND (sqlc.arg(project_name)::text = '' OR p.name = sqlc.arg(project_name))
AND (EXISTS (SELECT 1 FROM users u WHERE u.id = sqlc.arg(actor_id) AND u.platform_role IN ('admin', 'analyst'))
OR EXISTS (SELECT 1 FROM members m JOIN roles r ON r.id = m.role_id WHERE m.project_id = p.id AND m.user_id = sqlc.arg(actor_id) AND (r.all_permissions OR r.can_read_project)))
ORDER BY p.created_at, p.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: LockProject :one
SELECT id, name, description FROM projects WHERE name = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: GetProjectAccess :one
SELECT u.platform_role, COALESCE(r.all_permissions, false)::boolean AS all_permissions,
COALESCE(r.can_read_project, false)::boolean AS can_read_project,
COALESCE(r.can_update_project, false)::boolean AS can_update_project,
COALESCE(r.can_delete_project, false)::boolean AS can_delete_project,
COALESCE(r.can_manage_members, false)::boolean AS can_manage_members,
(m.id IS NOT NULL)::boolean AS is_member
FROM users u LEFT JOIN members m ON m.user_id = u.id AND m.project_id = sqlc.arg(project_id)
LEFT JOIN roles r ON r.id = m.role_id WHERE u.id = sqlc.arg(actor_id);

-- name: GetProjectTokenGrant :one
SELECT p.name,
CASE WHEN u.platform_role = 'admin' THEN 'admin' WHEN m.id IS NULL THEN 'analyst' ELSE r.name END::text AS role_name,
(u.platform_role = 'admin' OR COALESCE(r.all_permissions, false))::boolean AS all_permissions,
(u.platform_role IN ('admin', 'analyst') OR COALESCE(r.all_permissions, false) OR COALESCE(r.can_read_project, false))::boolean AS can_read_project,
(u.platform_role = 'admin' OR COALESCE(r.all_permissions, false) OR COALESCE(r.can_update_project, false))::boolean AS can_update_project,
(u.platform_role = 'admin' OR COALESCE(r.all_permissions, false) OR COALESCE(r.can_delete_project, false))::boolean AS can_delete_project,
(u.platform_role = 'admin' OR COALESCE(r.all_permissions, false) OR COALESCE(r.can_manage_members, false))::boolean AS can_manage_members,
(u.platform_role = 'admin' OR COALESCE(r.all_permissions, false) OR COALESCE(r.can_create_model, false))::boolean AS can_create_model,
(u.platform_role = 'admin' OR COALESCE(r.all_permissions, false) OR COALESCE(r.can_read_model, false))::boolean AS can_read_model,
(u.platform_role = 'admin' OR COALESCE(r.all_permissions, false) OR COALESCE(r.can_update_model, false))::boolean AS can_update_model,
(u.platform_role = 'admin' OR COALESCE(r.all_permissions, false) OR COALESCE(r.can_delete_model, false))::boolean AS can_delete_model,
(u.platform_role = 'admin' OR COALESCE(r.all_permissions, false) OR COALESCE(r.can_start_model, false))::boolean AS can_start_model,
(u.platform_role = 'admin' OR COALESCE(r.all_permissions, false) OR COALESCE(r.can_stop_model, false))::boolean AS can_stop_model
FROM projects p JOIN users u ON u.id = sqlc.arg(actor_id)
LEFT JOIN members m ON m.project_id = p.id AND m.user_id = u.id
LEFT JOIN roles r ON r.id = m.role_id
WHERE p.name = sqlc.arg(project_name) AND p.deleted_at IS NULL
AND (u.platform_role IN ('admin', 'analyst') OR m.id IS NOT NULL);

-- name: UpdateProjectDescription :one
UPDATE projects SET description = $2, updated_at = $3 WHERE id = $1 RETURNING id, name, description;

-- name: SoftDeleteProject :exec
UPDATE projects SET deleted_at = $2, updated_at = $2 WHERE id = $1;

-- name: GetRoleSystemKey :one
SELECT COALESCE(system_key, '')::text FROM roles WHERE id = $1;

-- name: GetMemberRoleKey :one
SELECT m.role_id, COALESCE(r.system_key, '')::text AS system_key FROM members m JOIN roles r ON r.id = m.role_id WHERE m.project_id = $1 AND m.user_id = $2;

-- name: CountTeamleads :one
SELECT count(*) FROM members m JOIN roles r ON r.id = m.role_id WHERE m.project_id = $1 AND r.system_key = 'teamlead';

-- name: UpsertMember :exec
INSERT INTO members (id, user_id, project_id, role_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (user_id, project_id) DO UPDATE SET role_id = EXCLUDED.role_id, updated_at = EXCLUDED.updated_at;

-- name: DeleteMember :execrows
DELETE FROM members WHERE project_id = $1 AND user_id = $2;

-- name: ListProjectMembers :many
SELECT u.id, u.email, u.firstname, u.lastname, r.id AS role_id, r.name AS role_name
FROM members m JOIN users u ON u.id = m.user_id JOIN roles r ON r.id = m.role_id
WHERE m.project_id = $1 ORDER BY m.created_at, m.id LIMIT $2 OFFSET $3;

-- name: AuditProjectChange :exec
INSERT INTO audit_events (id, actor_id, action, object_type, object_id, created_at, subject_id, details)
VALUES ($1, $2, $3, 'project', $4, $5, $6, $7);
