package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const MaxAttempts = 3

var (
	ErrLeaseLost = errors.New("job execution lease no longer belongs to this worker")
	ErrNotReady  = errors.New("job is not completed")
)

type WorkJob struct {
	ID       string
	UserID   string
	Filename string
	InputKey string
	Attempt  int
	Token    string
}

type ClaimState string

const (
	Claimed  ClaimState = "claimed"
	Busy     ClaimState = "busy"
	Finished ClaimState = "finished"
	Missing  ClaimState = "missing"
)

// ClaimJob atomically claims a queued job or an expired execution lease.
// Completed and terminally failed jobs are never executed again.
func (p *Postgres) ClaimJob(ctx context.Context, jobID, token string) (WorkJob, ClaimState, error) {
	job := WorkJob{ID: jobID, Token: token}
	err := p.DB.QueryRowContext(ctx, `UPDATE video_jobs SET
		status = 'processing',
		attempt_count = attempt_count + 1,
		started_at = COALESCE(started_at, NOW()),
		updated_at = NOW(),
		lease_token = $2::uuid,
		lease_expires_at = NOW() + INTERVAL '120 seconds',
		error_message = NULL
		WHERE id = $1::uuid AND
		(status = 'queued' OR (status = 'processing' AND
		 (lease_expires_at IS NULL OR lease_expires_at <= NOW())))
		RETURNING user_id::text, original_filename, input_object_key, attempt_count`,
		jobID, token).Scan(&job.UserID, &job.Filename, &job.InputKey, &job.Attempt)
	if err == nil {
		return job, Claimed, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return WorkJob{}, "", fmt.Errorf("claim video job: %w", err)
	}
	var status string
	err = p.DB.QueryRowContext(ctx,
		"SELECT status FROM video_jobs WHERE id = $1::uuid", jobID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkJob{}, Missing, nil
	}
	if err != nil {
		return WorkJob{}, "", err
	}
	if status == "completed" || status == "failed" {
		return WorkJob{}, Finished, nil
	}
	return WorkJob{}, Busy, nil
}

func (p *Postgres) RenewLease(ctx context.Context, jobID, token string) error {
	r, err := p.DB.ExecContext(ctx, `UPDATE video_jobs SET
		lease_expires_at = NOW() + INTERVAL '120 seconds',
		updated_at = NOW()
		WHERE id = $1::uuid AND lease_token = $2::uuid AND status = 'processing'`,
		jobID, token)
	return requireOwner(r, err)
}

func (p *Postgres) CompleteJob(ctx context.Context, jobID, token, objectKey string) error {
	r, err := p.DB.ExecContext(ctx, `UPDATE video_jobs SET
		status='completed', output_object_key=$3, completed_at=NOW(),
		lease_expires_at=NULL, lease_token=NULL, error_message=NULL,
		updated_at=NOW()
		WHERE id=$1::uuid AND lease_token=$2::uuid AND status='processing'`,
		jobID, token, objectKey)
	return requireOwner(r, err)
}

func (p *Postgres) FailOrRetryJob(ctx context.Context, jobID, token, reason string, terminal bool) error {
	target := "queued"
	if terminal { target = "failed" }
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()

	var userID string
	err = tx.QueryRowContext(ctx, `UPDATE video_jobs SET
		status=$3, error_message=$4, updated_at=NOW(),
		completed_at=CASE WHEN $3='failed' THEN NOW() ELSE NULL END,
		lease_token=NULL, lease_expires_at=NULL
		WHERE id=$1::uuid AND lease_token=$2::uuid AND status='processing'
		RETURNING user_id::text`, jobID, token, target, reason).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) { return ErrLeaseLost }
	if err != nil { return err }
	if terminal {
		_, err = tx.ExecContext(ctx, `INSERT INTO user_notifications
			(user_id, job_id, category, message)
			VALUES ($1::uuid, $2::uuid, 'processing_failed', $3)
			ON CONFLICT (job_id, category) DO NOTHING`,
			userID, jobID, "Não foi possível processar seu vídeo. Consulte o trabalho para detalhes.")
		if err != nil { return fmt.Errorf("persist failure notification: %w", err) }
	}
	return tx.Commit()
}

func requireOwner(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrLeaseLost
	}
	return nil
}

// Download metadata is never returned without matching both job id and owner id.
func (p *Postgres) GetCompletedOutput(ctx context.Context, userID, jobID string) (string, error) {
	var status string
	var key sql.NullString
	err := p.DB.QueryRowContext(ctx, `SELECT status, output_object_key
		FROM video_jobs WHERE id=$1::uuid AND user_id=$2::uuid`, jobID, userID).
		Scan(&status, &key)
	if err != nil {
		return "", err
	}
	if status != "completed" || !key.Valid || key.String == "" {
		return "", ErrNotReady
	}
	return key.String, nil
}
