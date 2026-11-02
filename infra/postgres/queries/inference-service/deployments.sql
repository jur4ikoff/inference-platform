-- name: ActiveDeploymentExists :one
SELECT EXISTS (
    SELECT 1 FROM deployments
    WHERE project = $1 AND model_id = $2
      AND status = ANY(sqlc.arg(statuses)::text[])
);

-- name: CreateDeployment :one
INSERT INTO deployments (
    id, container_id, model_id, project, username, name, deployment_name, status, spec
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, container_id, model_id, project, username, name, deployment_name, status, spec;

-- name: GetDeploymentByID :one
SELECT id, container_id, model_id, project, username, name, deployment_name, status, spec
FROM deployments WHERE id = $1;

-- name: GetDeploymentByName :one
SELECT id, container_id, model_id, project, username, name, deployment_name, status, spec
FROM deployments WHERE name = $1;

-- name: ListDeploymentStates :many
SELECT id, container_id, status, project, deployment_name, name FROM deployments ORDER BY id;

-- name: ListDeployments :many
SELECT id, container_id, model_id, project, username, name, deployment_name, status, spec
FROM deployments
WHERE status = ANY(sqlc.arg(statuses)::text[])
ORDER BY id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListAllDeployments :many
SELECT id, container_id, model_id, project, username, name, deployment_name, status, spec
FROM deployments
ORDER BY id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: GetDeploymentState :one
SELECT id, container_id, status, project, deployment_name, name FROM deployments WHERE container_id = $1;

-- name: UpdateDeploymentStatus :execrows
UPDATE deployments
SET status = sqlc.arg(new_status)
WHERE id = sqlc.arg(id)
  AND container_id = sqlc.arg(container_id)
  AND status = sqlc.arg(expected_status)
  AND status <> sqlc.arg(new_status);
