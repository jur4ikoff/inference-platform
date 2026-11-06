-- name: CreateUser :one
INSERT INTO users (id,email,login,password_hash,created_at,updated_at,firstname,lastname) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;

-- name: FindUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower($1);

-- name: FindUserByLogin :one
SELECT * FROM users WHERE lower(login) = lower($1);

-- name: FindUserByEmailForUpdate :one
SELECT * FROM users WHERE lower(email) = lower($1) FOR UPDATE;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: ListUsers :many
SELECT id,email,firstname,lastname FROM users ORDER BY created_at,id LIMIT $1 OFFSET $2;

-- name: UpdateUser :one
UPDATE users SET email=COALESCE(sqlc.narg(email),email), password_hash=COALESCE(sqlc.narg(password_hash),password_hash), firstname=COALESCE(sqlc.narg(firstname),firstname), lastname=COALESCE(sqlc.narg(lastname),lastname), updated_at=sqlc.arg(updated_at), auth_version=auth_version + CASE WHEN sqlc.narg(email)::text IS NOT NULL OR sqlc.narg(password_hash)::text IS NOT NULL THEN 1 ELSE 0 END WHERE id=sqlc.arg(id) RETURNING *;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id=$1;

-- name: GetVisibleUser :one
SELECT u.id,u.email,u.firstname,u.lastname, COALESCE(CASE WHEN u.id=sqlc.arg(actor_id) OR EXISTS (SELECT 1 FROM users admin_actor WHERE admin_actor.id=sqlc.arg(actor_id) AND admin_actor.platform_role='admin') THEN u.platform_role END, '')::text AS visible_platform_role FROM users u WHERE u.id=sqlc.arg(target_id) AND (u.id=sqlc.arg(actor_id) OR EXISTS (SELECT 1 FROM users a WHERE a.id=sqlc.arg(actor_id) AND a.platform_role='admin') OR EXISTS (SELECT 1 FROM members a JOIN members b ON a.project_id=b.project_id JOIN projects p ON p.id=a.project_id WHERE a.user_id=sqlc.arg(actor_id) AND b.user_id=u.id AND p.deleted_at IS NULL));

-- name: ListVisibleUsers :many
SELECT u.id,u.email,u.firstname,u.lastname, COALESCE(CASE WHEN u.id=sqlc.arg(actor_id) OR EXISTS (SELECT 1 FROM users admin_actor WHERE admin_actor.id=sqlc.arg(actor_id) AND admin_actor.platform_role='admin') THEN u.platform_role END, '')::text AS visible_platform_role FROM users u WHERE u.id=sqlc.arg(actor_id) OR EXISTS (SELECT 1 FROM users a WHERE a.id=sqlc.arg(actor_id) AND a.platform_role='admin') OR EXISTS (SELECT 1 FROM members a JOIN members b ON a.project_id=b.project_id JOIN projects p ON p.id=a.project_id WHERE a.user_id=sqlc.arg(actor_id) AND b.user_id=u.id AND p.deleted_at IS NULL) ORDER BY u.created_at,u.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: LockAdminRegistry :exec
SELECT pg_advisory_xact_lock(19843315);

-- name: LockUser :one
SELECT platform_role FROM users WHERE id=$1 FOR UPDATE;

-- name: CountOtherAdmins :one
SELECT count(*) FROM users WHERE platform_role='admin' AND id<>$1;

-- name: CountAdmins :one
SELECT count(*) FROM users WHERE platform_role = 'admin';

-- name: CreateAdminUser :one
INSERT INTO users (id, email, login, password_hash, platform_role, created_at, updated_at, firstname, lastname)
VALUES ($1, $2, $3, $4, 'admin', $5, $6, $7, $8) RETURNING *;

-- name: LockUserProjects :many
SELECT p.id FROM projects p JOIN members m ON m.project_id=p.id WHERE m.user_id=$1 AND p.deleted_at IS NULL ORDER BY p.id FOR UPDATE OF p;

-- name: CountOrphanedProjects :one
SELECT count(*) FROM members m JOIN projects p ON p.id=m.project_id JOIN roles r ON r.id=m.role_id WHERE m.user_id=$1 AND p.deleted_at IS NULL AND r.system_key='teamlead' AND NOT EXISTS (SELECT 1 FROM members other JOIN roles r2 ON r2.id=other.role_id WHERE other.project_id=m.project_id AND other.user_id<>m.user_id AND r2.system_key='teamlead');

-- name: AuditUserChange :exec
INSERT INTO audit_events (id, actor_id, action, object_type, object_id, created_at, details)
VALUES (sqlc.arg(id), sqlc.narg(actor_id), sqlc.arg(action), 'user', sqlc.arg(object_id), sqlc.arg(created_at), sqlc.arg(details));

-- name: GetAuthState :one
SELECT id, platform_role, auth_version FROM users WHERE id=$1;

-- name: UpdatePlatformRole :one
UPDATE users SET platform_role = $2, auth_version = auth_version + 1, updated_at = $3 WHERE id = $1 RETURNING *;

-- name: UpdatePasswordHashIfCurrent :one
UPDATE users
SET password_hash = sqlc.arg(new_hash), auth_version = auth_version + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND password_hash = sqlc.arg(previous_hash)
RETURNING id, platform_role, auth_version;
