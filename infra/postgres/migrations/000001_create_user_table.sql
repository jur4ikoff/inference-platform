CREATE TABLE users (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    firstname TEXT NOT NULL,
    lastname TEXT NOT NULL,
    login TEXT NOT NULL,
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    platform_role TEXT NOT NULL DEFAULT 'user',
    auth_version BIGINT NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX users_email_lower_unique_idx ON users (lower(email));
CREATE UNIQUE INDEX users_login_lower_unique_idx ON users (lower(login));

CREATE TABLE audit_events (
    id UUID PRIMARY KEY,
    actor_id UUID,
    action TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id UUID NOT NULL,
    subject_id UUID,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL,
    request_id TEXT NOT NULL DEFAULT ''
);
