-- Existing deployments have no reliable creation time; leave it NULL.
ALTER TABLE deployments ADD COLUMN created_at TIMESTAMPTZ;
ALTER TABLE deployments ALTER COLUMN created_at SET DEFAULT clock_timestamp();
