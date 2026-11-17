package store

import (
	"context"
	"database/sql"
	"fmt"
)

func (p *Postgres) CreateQueuedJob(ctx context.Context, userID, jobID, filename, objectKey string) (Job, error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()

	job := Job{ID: jobID, Filename: filename, Status: "queued"}
	err = tx.QueryRowContext(ctx, `INSERT INTO video_jobs
		(id, user_id, original_filename, status, input_object_key)
		VALUES ($1, $2, $3, 'queued', $4)
		RETURNING created_at`, jobID, userID, filename, objectKey).Scan(&job.CreatedAt)
	if err != nil {
		return Job{}, fmt.Errorf("insert video job: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		"INSERT INTO video_job_outbox (job_id) VALUES ($1)", jobID)
	if err != nil {
		return Job{}, fmt.Errorf("insert dispatch outbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Job{}, fmt.Errorf("commit video job: %w", err)
	}
	return job, nil
}

type JobPublisher interface {
	PublishJob(context.Context, string) error
}

// PublishPending holds row-level locks until it has confirmed broker delivery
// and recorded published_at. Crashes can cause duplicates but not lost jobs.
// Workers must be idempotent; the consumer will be delivered in PR #4.
func (p *Postgres) PublishPending(ctx context.Context, publisher JobPublisher) (int, error) {
	tx, err := p.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id, job_id::text FROM video_job_outbox
		WHERE published_at IS NULL ORDER BY id LIMIT 25
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	type event struct {
		id    int64
		jobID string
	}
	events := make([]event, 0, 25)
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.id, &e.jobID); err != nil {
			rows.Close()
			return 0, err
		}
		events = append(events, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, e := range events {
		if err := publisher.PublishJob(ctx, e.jobID); err != nil {
			return 0, fmt.Errorf("publish job %s: %w", e.jobID, err)
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE video_job_outbox SET published_at = NOW() WHERE id = $1", e.id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(events), nil
}
