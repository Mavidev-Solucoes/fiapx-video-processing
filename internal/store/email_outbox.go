package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

const maxEmailAttempts = 8

type EmailSender interface {
	SendFailure(context.Context, string, string, int64) error
}

type emailDelivery struct {
	id             int64
	notificationID int64
	recipient      string
	jobID          string
	attempts       int
}

// DispatchFailureEmails locks a short batch of email intents. The row remains
// pending until the SMTP server confirms DATA. A crash after sending but
// before database commit may produce a duplicate email (at-least-once).
func (p *Postgres) DispatchFailureEmails(ctx context.Context, sender EmailSender) (int, error) {
	tx, err := p.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT o.id, o.notification_id, u.email, n.job_id::text, o.attempts
		FROM notification_email_outbox o
		JOIN user_notifications n ON n.id=o.notification_id
		JOIN users u ON u.id=n.user_id
		WHERE o.sent_at IS NULL AND o.abandoned_at IS NULL
		  AND o.next_attempt_at <= NOW()
		ORDER BY o.id LIMIT 5 FOR UPDATE OF o SKIP LOCKED`)
	if err != nil {
		return 0, fmt.Errorf("select email outbox: %w", err)
	}
	batch := make([]emailDelivery, 0, 5)
	for rows.Next() {
		var d emailDelivery
		if err := rows.Scan(&d.id, &d.notificationID, &d.recipient, &d.jobID, &d.attempts); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}

	delivered := 0
	for _, delivery := range batch {
		sendCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		sendErr := sender.SendFailure(sendCtx, delivery.recipient, delivery.jobID, delivery.notificationID)
		cancel()

		if sendErr == nil {
			_, err = tx.ExecContext(ctx, `UPDATE notification_email_outbox
				SET sent_at=NOW(), last_error=NULL WHERE id=$1`, delivery.id)
			if err != nil {
				return 0, err
			}
			delivered++
			continue
		}

		// Never log recipient addresses or confidential SMTP responses.
		log.Printf("email notification %d delivery failed (attempt %d)",
			delivery.notificationID, delivery.attempts+1)
		newAttempts := delivery.attempts + 1
		abandon := newAttempts >= maxEmailAttempts
		delay := retryEmailDelay(newAttempts)
		_, err = tx.ExecContext(ctx, `UPDATE notification_email_outbox
			SET attempts=$2, next_attempt_at=NOW()+($3 * INTERVAL '1 second'),
			abandoned_at=CASE WHEN $4 THEN NOW() ELSE NULL END,
			last_error='SMTP delivery failed'
			WHERE id=$1`, delivery.id, newAttempts, int(delay.Seconds()), abandon)
		if err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return delivered, nil
}

func retryEmailDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(5*(1<<(attempt-1))) * time.Second
}
