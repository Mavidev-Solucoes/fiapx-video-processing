-- User-visible notifications for terminal video processing failures.
-- A unique key makes redelivered messages unable to generate duplicates.
CREATE TABLE IF NOT EXISTS user_notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_id UUID NOT NULL REFERENCES video_jobs(id) ON DELETE CASCADE,
    category TEXT NOT NULL DEFAULT 'processing_failed'
       CHECK (category IN ('processing_failed')),
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at TIMESTAMPTZ,
    UNIQUE (job_id, category)
);
CREATE INDEX IF NOT EXISTS ix_user_notifications_owner_created
    ON user_notifications(user_id, created_at DESC, id DESC);
