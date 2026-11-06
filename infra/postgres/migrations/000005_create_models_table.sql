CREATE TABLE models (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    username TEXT NOT NULL,
    project TEXT NOT NULL,
    spec JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX models_project_idx ON models (project);
CREATE INDEX models_username_idx ON models (username);
CREATE UNIQUE INDEX models_project_name_key ON models (project, name);
