-- Atomic email delivery intent for each terminal processing-failure notification.
-- An SMTP relay can fail/restart without losing a committed email request.
CREATE TABLE IF NOT EXISTS notification_email_outbox (
    id BIGSERIAL PRIMARY KEY,
    notification_id BIGINT NOT NULL UNIQUE REFERENCES user_notifications(id) ON DELETE CASCADE,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ,
    abandoned_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS ix_notification_email_outbox_pending
    ON notification_email_outbox(next_attempt_at, id)
    WHERE sent_at IS NULL AND abandoned_at IS NULL;
