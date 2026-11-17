-- Idempotent migration for both existing volumes and fresh local databases.
-- A job and its outbox entry are written in the same SQL transaction.
CREATE TABLE IF NOT EXISTS video_job_outbox (
    id BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL UNIQUE REFERENCES video_jobs(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS ix_video_job_outbox_unpublished
    ON video_job_outbox(id) WHERE published_at IS NULL;
