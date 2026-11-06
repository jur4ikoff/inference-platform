CREATE TABLE projects (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    deleted_at TIMESTAMPTZ
);
