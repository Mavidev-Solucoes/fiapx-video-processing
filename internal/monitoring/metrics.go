package monitoring

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Snapshot contains aggregate operational information only. It never exposes
// user IDs, video names, credentials, or object storage paths.
type Snapshot struct {
	Queued                  int64
	Processing              int64
	Completed               int64
	Failed                  int64
	OutboxPending           int64
	UnreadNotifications     int64
	Attempts                int64
	OldestQueuedSeconds     float64
	OldestProcessingSeconds float64
}

type Source interface {
	ReadSnapshot(context.Context) (Snapshot, error)
}

type Postgres struct {
	DB *sql.DB
}

func (p Postgres) ReadSnapshot(ctx context.Context) (Snapshot, error) {
	var s Snapshot
	err := p.DB.QueryRowContext(ctx, `SELECT
		COUNT(*) FILTER (WHERE status='queued'),
		COUNT(*) FILTER (WHERE status='processing'),
		COUNT(*) FILTER (WHERE status='completed'),
		COUNT(*) FILTER (WHERE status='failed'),
		COALESCE(SUM(attempt_count), 0),
		COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at) FILTER (WHERE status='queued'))), 0)::float8,
		COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(started_at) FILTER (WHERE status='processing'))), 0)::float8
		FROM video_jobs`).Scan(
		&s.Queued, &s.Processing, &s.Completed, &s.Failed, &s.Attempts,
		&s.OldestQueuedSeconds, &s.OldestProcessingSeconds,
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("query job metrics: %w", err)
	}
	if err := p.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM video_job_outbox WHERE published_at IS NULL").
		Scan(&s.OutboxPending); err != nil {
		return Snapshot{}, fmt.Errorf("query outbox metrics: %w", err)
	}
	if err := p.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_notifications WHERE read_at IS NULL").
		Scan(&s.UnreadNotifications); err != nil {
		return Snapshot{}, fmt.Errorf("query notification metrics: %w", err)
	}
	return s, nil
}

// Handler is bound to a dedicated internal port, not the user-facing API.
// PostgreSQL failures produce 503 so Prometheus marks the scrape as failed.
func Handler(source Source) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		s, err := source.ReadSnapshot(ctx)
		if err != nil {
			http.Error(w, "metrics temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		var b strings.Builder
		b.WriteString("# HELP fiapx_jobs Current number of video jobs by status.\n")
		b.WriteString("# TYPE fiapx_jobs gauge\n")
		fmt.Fprintf(&b, "fiapx_jobs{status=\"queued\"} %d\n", s.Queued)
		fmt.Fprintf(&b, "fiapx_jobs{status=\"processing\"} %d\n", s.Processing)
		fmt.Fprintf(&b, "fiapx_jobs{status=\"completed\"} %d\n", s.Completed)
		fmt.Fprintf(&b, "fiapx_jobs{status=\"failed\"} %d\n", s.Failed)
		writeGauge(&b, "fiapx_outbox_pending", "Unpublished job events in the transactional outbox.", s.OutboxPending)
		writeGauge(&b, "fiapx_notifications_unread", "Unread processing failure notifications.", s.UnreadNotifications)
		writeGauge(&b, "fiapx_job_attempts_sum", "Sum of worker attempts recorded by jobs.", s.Attempts)
		writeFloatGauge(&b, "fiapx_oldest_queued_age_seconds", "Age in seconds of the oldest queued job.", s.OldestQueuedSeconds)
		writeFloatGauge(&b, "fiapx_oldest_processing_age_seconds", "Age in seconds of the oldest processing job.", s.OldestProcessingSeconds)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(b.String()))
	})
}

func writeGauge(b *strings.Builder, name, help string, value int64) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", name, help, name, name, value)
}

func writeFloatGauge(b *strings.Builder, name, help string, value float64) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s gauge\n%s %.3f\n", name, help, name, name, value)
}
