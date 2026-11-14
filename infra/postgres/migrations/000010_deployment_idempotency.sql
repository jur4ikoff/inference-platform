CREATE TABLE deployment_idempotency (
    owner_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    project TEXT NOT NULL,
    model_id UUID NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'completed', 'failed')),
    response JSONB,
    error_status INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_id, idempotency_key)
);

GRANT SELECT, INSERT, UPDATE ON deployment_idempotency TO model_service_rw;
