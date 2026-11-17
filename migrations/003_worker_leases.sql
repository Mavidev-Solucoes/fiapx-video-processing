-- Claims and heartbeats prevent a second worker from executing the same job.
-- The lease token also fences stale workers from committing after ownership changes.
ALTER TABLE video_jobs ADD COLUMN IF NOT EXISTS lease_token UUID;
ALTER TABLE video_jobs ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS ix_video_jobs_expired_lease
 ON video_jobs(lease_expires_at) WHERE status = 'processing';
