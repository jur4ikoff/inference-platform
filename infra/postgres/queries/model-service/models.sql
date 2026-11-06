-- name: CreateModel :one
INSERT INTO models (
    id, created_at, updated_at, name, description, username, project, spec
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, created_at, updated_at, name, description, username, project, spec;

-- name: GetModel :one
SELECT id, created_at, updated_at, name, description, username, project, spec
FROM models
WHERE project = $1 AND id = $2;

-- name: ListModels :many
SELECT id, created_at, updated_at, name, description, username, project, spec
FROM models
WHERE project = $1
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)::bigint OFFSET sqlc.arg(page_offset)::bigint;

-- name: UpdateModel :one
UPDATE models
SET name = COALESCE(sqlc.narg(name)::text, name),
    description = COALESCE(sqlc.narg(description)::text, description),
    spec = COALESCE(sqlc.narg(spec)::jsonb, spec),
    updated_at = sqlc.arg(updated_at)
WHERE project = sqlc.arg(project) AND id = sqlc.arg(id)
RETURNING id, created_at, updated_at, name, description, username, project, spec;

-- name: DeleteModel :execrows
DELETE FROM models
WHERE project = $1 AND id = $2;
