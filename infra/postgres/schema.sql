-- Schema used by sqlc to check queries and generate Go types.
-- Apply database changes through separate files in migrations/.

CREATE TABLE deployments (
    id UUID PRIMARY KEY,
    container_id TEXT NOT NULL UNIQUE,
    model_id UUID NOT NULL,
    project TEXT NOT NULL,
    username TEXT NOT NULL,
    name TEXT NOT NULL UNIQUE,
    deployment_name TEXT NOT NULL,
    status TEXT NOT NULL,
    spec JSONB NOT NULL CHECK (jsonb_typeof(spec) = 'object')
);

CREATE UNIQUE INDEX deployments_project_model_id_key ON deployments (project, model_id)
    WHERE status IN ('PENDING', 'STARTING', 'STARTED', 'RUNNING', 'STOPPING');
CREATE UNIQUE INDEX deployments_project_deployment_name_key ON deployments (project, deployment_name)
    WHERE status IN ('PENDING', 'STARTING', 'STARTED', 'RUNNING', 'STOPPING');

CREATE INDEX deployments_model_id_idx ON deployments (model_id);
CREATE INDEX deployments_project_idx ON deployments (project);
