-- Migration 000007 may already have been applied without updated_at.
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;
ALTER TABLE deployments ALTER COLUMN updated_at SET DEFAULT clock_timestamp();
CREATE INDEX IF NOT EXISTS deployments_created_at_idx ON deployments (created_at DESC NULLS LAST, id DESC);
