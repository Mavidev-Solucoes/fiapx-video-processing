package store

import (
	"context"
	"database/sql"
	"time"
)

type Notification struct {
	ID        int64      `json:"id"`
	JobID     string     `json:"jobId"`
	Category  string     `json:"category"`
	Message   string     `json:"message"`
	CreatedAt time.Time  `json:"createdAt"`
	ReadAt    *time.Time `json:"readAt,omitempty"`
}

func (p *Postgres) ListNotifications(ctx context.Context, userID string) ([]Notification, error) {
	rows, err := p.DB.QueryContext(ctx, `SELECT id, job_id::text, category, message, created_at, read_at
		FROM user_notifications WHERE user_id=$1::uuid
		ORDER BY created_at DESC, id DESC LIMIT 100`, userID)
	if err != nil { return nil, err }
	defer rows.Close()
	out := make([]Notification, 0)
	for rows.Next() {
		var n Notification
		var readAt sql.NullTime
		if err := rows.Scan(&n.ID, &n.JobID, &n.Category, &n.Message, &n.CreatedAt, &readAt); err != nil { return nil, err }
		if readAt.Valid { n.ReadAt = &readAt.Time }
		out = append(out, n)
	}
	return out, rows.Err()
}

func (p *Postgres) MarkNotificationRead(ctx context.Context, userID string, id int64) error {
	var updated int64
	return p.DB.QueryRowContext(ctx, `UPDATE user_notifications
		SET read_at=COALESCE(read_at, NOW())
		WHERE id=$1 AND user_id=$2::uuid RETURNING id`, id, userID).Scan(&updated)
}
