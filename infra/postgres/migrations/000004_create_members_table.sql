CREATE TABLE members (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    UNIQUE (user_id, project_id)
);

CREATE INDEX members_project_id_idx ON members (project_id);
CREATE INDEX members_role_id_idx ON members (role_id);
