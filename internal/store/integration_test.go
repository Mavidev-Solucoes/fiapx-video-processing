package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/identity"
)

func integrationPostgres(t *testing.T) *Postgres {
	t.Helper()
	dsn := os.Getenv("FIAPX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("FIAPX_TEST_DATABASE_URL not configured; CI starts a real PostgreSQL instance")
	}
	database, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open integration PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = database.DB.Close() })
	return database
}

func testUUID(t *testing.T) string {
	t.Helper()
	id, err := identity.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func insertTestUser(t *testing.T, database *Postgres) User {
	t.Helper()
	u := User{ID: testUUID(t), PasswordHash: "integration-test-hash"}
	u.Email = fmt.Sprintf("ci-%s@example.test", u.ID)
	if err := database.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := database.DB.ExecContext(ctx,
			"DELETE FROM video_jobs WHERE user_id=$1::uuid", u.ID); err != nil {
			t.Errorf("clean up jobs: %v", err)
		}
		if _, err := database.DB.ExecContext(ctx,
			"DELETE FROM users WHERE id=$1::uuid", u.ID); err != nil {
			t.Errorf("clean up user: %v", err)
		}
	})
	return u
}

func insertTestJob(t *testing.T, database *Postgres, user User) Job {
	t.Helper()
	id := testUUID(t)
	job, err := database.CreateQueuedJob(context.Background(), user.ID, id, "test-video.mp4", "input/"+id)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if job.ID != id || job.Status != "queued" || job.CreatedAt.IsZero() {
		t.Fatalf("unexpected queued job: %+v", job)
	}
	return job
}

type integrationPublisher struct {
	ids []string
	err error
}

func (p *integrationPublisher) PublishJob(_ context.Context, id string) error {
	if p.err != nil {
		return p.err
	}
	p.ids = append(p.ids, id)
	return nil
}

type integrationMailer struct {
	err  error
	sent []string
}

func (m *integrationMailer) SendFailure(_ context.Context, to, id string, notificationID int64) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, fmt.Sprintf("%s:%s:%d", to, id, notificationID))
	return nil
}

func TestDatabaseUsersJobsAndOwnership(t *testing.T) {
	db := integrationPostgres(t)
	ctx := context.Background()
	alice := insertTestUser(t, db)
	bob := insertTestUser(t, db)

	if err := db.CreateUser(ctx, User{
		ID: testUUID(t), Email: alice.Email, PasswordHash: "some-hash",
	}); !errors.Is(err, ErrEmailExists) {
		t.Fatalf("duplicate email must be rejected: %v", err)
	}
	gotEmail, err := db.FindUserByEmail(ctx, alice.Email)
	if err != nil || gotEmail.ID != alice.ID {
		t.Fatalf("lookup by email: %+v, %v", gotEmail, err)
	}
	gotID, err := db.FindUserByID(ctx, bob.ID)
	if err != nil || gotID.Email != bob.Email {
		t.Fatalf("lookup by id: %+v, %v", gotID, err)
	}
	if _, err := db.FindUserByEmail(ctx, "nobody@example.test"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing user by email: %v", err)
	}
	if _, err := db.FindUserByID(ctx, testUUID(t)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing user by id: %v", err)
	}

	job := insertTestJob(t, db, alice)
	ownerJob, err := db.GetJobForUser(ctx, alice.ID, job.ID)
	if err != nil || ownerJob.Filename != "test-video.mp4" || ownerJob.Status != "queued" {
		t.Fatalf("owner job view: %+v, %v", ownerJob, err)
	}
	if _, err := db.GetJobForUser(ctx, bob.ID, job.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user job leaked: %v", err)
	}
	if _, err := db.GetCompletedOutput(ctx, bob.ID, job.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user object key leaked: %v", err)
	}
	if _, err := db.GetCompletedOutput(ctx, alice.ID, job.ID); !errors.Is(err, ErrNotReady) {
		t.Fatalf("unfinished ZIP available: %v", err)
	}
	aliceJobs, err := db.ListJobsForUser(ctx, alice.ID)
	if err != nil || len(aliceJobs) != 1 || aliceJobs[0].ID != job.ID {
		t.Fatalf("owner jobs: %+v, %v", aliceJobs, err)
	}
	bobJobs, err := db.ListJobsForUser(ctx, bob.ID)
	if err != nil || len(bobJobs) != 0 {
		t.Fatalf("cross-user listing: %+v, %v", bobJobs, err)
	}
	if _, err := db.CreateQueuedJob(ctx, alice.ID, job.ID, "duplicate.mp4", "input/duplicate"); err == nil {
		t.Fatal("duplicate job ID was accepted")
	}
	if _, err := db.CreateQueuedJob(ctx, "invalid-uuid", testUUID(t), "invalid.mp4", "input/invalid"); err == nil {
		t.Fatal("invalid user was accepted")
	}
}

func TestDatabaseTransactionalOutboxAndPublisherRecovery(t *testing.T) {
	db := integrationPostgres(t)
	ctx := context.Background()
	user := insertTestUser(t, db)
	job := insertTestJob(t, db, user)
	publisher := &integrationPublisher{err: errors.New("broker unavailable")}
	if count, err := db.PublishPending(ctx, publisher); err == nil || count != 0 {
		t.Fatalf("broker failure should keep outbox pending: %d %v", count, err)
	}
	var pending bool
	if err := db.DB.QueryRowContext(ctx, "SELECT published_at IS NULL FROM video_job_outbox WHERE job_id=$1::uuid", job.ID).Scan(&pending); err != nil || !pending {
		t.Fatalf("pending outbox event lost: %v (pending=%v)", err, pending)
	}
	publisher.err = nil
	count, err := db.PublishPending(ctx, publisher)
	if err != nil || count != 1 || len(publisher.ids) != 1 || publisher.ids[0] != job.ID {
		t.Fatalf("pending dispatch: count=%d ids=%v err=%v", count, publisher.ids, err)
	}
	count, err = db.PublishPending(ctx, publisher)
	if err != nil || count != 0 || len(publisher.ids) != 1 {
		t.Fatalf("published job must not republish: count=%d err=%v", count, err)
	}
}

func TestDatabaseLeaseRecoveryAndJobLifecycle(t *testing.T) {
	db := integrationPostgres(t)
	ctx := context.Background()
	user := insertTestUser(t, db)
	job := insertTestJob(t, db, user)
	token1, token2, token3 := testUUID(t), testUUID(t), testUUID(t)

	claim, state, err := db.ClaimJob(ctx, job.ID, token1)
	if err != nil || state != Claimed || claim.UserID != user.ID || claim.Attempt != 1 {
		t.Fatalf("first claim: %+v %s %v", claim, state, err)
	}
	_, state, err = db.ClaimJob(ctx, job.ID, token2)
	if err != nil || state != Busy {
		t.Fatalf("active lease should be busy: %s %v", state, err)
	}
	if err := db.RenewLease(ctx, job.ID, token1); err != nil {
		t.Fatalf("renew valid lease: %v", err)
	}
	if err := db.RenewLease(ctx, job.ID, token2); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale token renewed lease: %v", err)
	}
	if _, state, err := db.ClaimJob(ctx, testUUID(t), token2); err != nil || state != Missing {
		t.Fatalf("unknown job claim: %s %v", state, err)
	}
	if _, _, err := db.ClaimJob(ctx, "invalid-uuid", token2); err == nil {
		t.Fatal("invalid job ID was accepted")
	}
	if _, err := db.DB.ExecContext(ctx, "UPDATE video_jobs SET lease_expires_at=NOW()-INTERVAL '5 seconds' WHERE id=$1::uuid", job.ID); err != nil {
		t.Fatal(err)
	}
	second, state, err := db.ClaimJob(ctx, job.ID, token2)
	if err != nil || state != Claimed || second.Attempt != 2 {
		t.Fatalf("expired lease reclaim: %+v %s %v", second, state, err)
	}
	if err := db.CompleteJob(ctx, job.ID, token1, "zip/stale.zip"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale worker completed job: %v", err)
	}
	if err := db.FailOrRetryJob(ctx, job.ID, token1, "stale", true); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale worker failed job: %v", err)
	}
	if err := db.FailOrRetryJob(ctx, job.ID, token2, "transient", false); err != nil {
		t.Fatalf("requeue transient failure: %v", err)
	}
	afterRetry, err := db.GetJobForUser(ctx, user.ID, job.ID)
	if err != nil || afterRetry.Status != "queued" || afterRetry.ErrorMessage != "transient" {
		t.Fatalf("retry state: %+v %v", afterRetry, err)
	}
	third, state, err := db.ClaimJob(ctx, job.ID, token3)
	if err != nil || state != Claimed || third.Attempt != 3 {
		t.Fatalf("third claim: %+v %s %v", third, state, err)
	}
	if err := db.CompleteJob(ctx, job.ID, token3, "outputs/final.zip"); err != nil {
		t.Fatalf("complete valid lease: %v", err)
	}
	key, err := db.GetCompletedOutput(ctx, user.ID, job.ID)
	if err != nil || key != "outputs/final.zip" {
		t.Fatalf("completed output: %q %v", key, err)
	}
	completed, err := db.GetJobForUser(ctx, user.ID, job.ID)
	if err != nil || completed.Status != "completed" || completed.CompletedAt == nil || completed.StartedAt == nil {
		t.Fatalf("complete timestamps: %+v %v", completed, err)
	}
	if _, state, err := db.ClaimJob(ctx, job.ID, token1); err != nil || state != Finished {
		t.Fatalf("finished job was claimed: %s %v", state, err)
	}
	if err := db.CompleteJob(ctx, job.ID, token3, "outputs/again.zip"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("repeated completion was accepted: %v", err)
	}
}

func TestDatabaseTerminalFailureAndEmailDelivery(t *testing.T) {
	db := integrationPostgres(t)
	ctx := context.Background()
	user := insertTestUser(t, db)
	other := insertTestUser(t, db)
	job := insertTestJob(t, db, user)
	token := testUUID(t)
	if _, state, err := db.ClaimJob(ctx, job.ID, token); err != nil || state != Claimed {
		t.Fatalf("claim: %s %v", state, err)
	}
	if err := db.FailOrRetryJob(ctx, job.ID, token, "bad codec", true); err != nil {
		t.Fatalf("terminal failure: %v", err)
	}
	if _, state, err := db.ClaimJob(ctx, job.ID, testUUID(t)); err != nil || state != Finished {
		t.Fatalf("failed job was reclaimed: %s %v", state, err)
	}
	if _, err := db.GetCompletedOutput(ctx, user.ID, job.ID); !errors.Is(err, ErrNotReady) {
		t.Fatalf("failed ZIP was accessible: %v", err)
	}
	notices, err := db.ListNotifications(ctx, user.ID)
	if err != nil || len(notices) != 1 || notices[0].JobID != job.ID || notices[0].ReadAt != nil {
		t.Fatalf("owner notification: %+v %v", notices, err)
	}
	if leaked, err := db.ListNotifications(ctx, other.ID); err != nil || len(leaked) != 0 {
		t.Fatalf("cross-user notifications leaked: %+v %v", leaked, err)
	}
	if err := db.MarkNotificationRead(ctx, other.ID, notices[0].ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user marked alert read: %v", err)
	}
	if err := db.MarkNotificationRead(ctx, user.ID, notices[0].ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	updated, err := db.ListNotifications(ctx, user.ID)
	if err != nil || len(updated) != 1 || updated[0].ReadAt == nil {
		t.Fatalf("read notification timestamp: %+v %v", updated, err)
	}
	failer := &integrationMailer{err: errors.New("SMTP relay unavailable")}
	if count, err := db.DispatchFailureEmails(ctx, failer); err != nil || count != 0 {
		t.Fatalf("SMTP failure must be retained: count=%d err=%v", count, err)
	}
	var attempts int
	if err := db.DB.QueryRowContext(ctx, `SELECT attempts FROM notification_email_outbox
		WHERE notification_id=$1`, notices[0].ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("retry attempt count: %d %v", attempts, err)
	}
	// Make the delayed message immediately eligible for deterministic testing.
	if _, err := db.DB.ExecContext(ctx, `UPDATE notification_email_outbox
		SET next_attempt_at=NOW()-INTERVAL '1 second' WHERE notification_id=$1`, notices[0].ID); err != nil {
		t.Fatal(err)
	}
	mailer := &integrationMailer{}
	count, err := db.DispatchFailureEmails(ctx, mailer)
	if err != nil || count != 1 || len(mailer.sent) != 1 {
		t.Fatalf("email recovery: count=%d emails=%v err=%v", count, mailer.sent, err)
	}
	if count, err := db.DispatchFailureEmails(ctx, mailer); err != nil || count != 0 {
		t.Fatalf("email delivered twice: count=%d err=%v", count, err)
	}
}

func TestDatabaseEmailExhaustionAndEmptyOutput(t *testing.T) {
	db := integrationPostgres(t)
	ctx := context.Background()
	user := insertTestUser(t, db)
	job := insertTestJob(t, db, user)
	token := testUUID(t)
	if _, _, err := db.ClaimJob(ctx, job.ID, token); err != nil {
		t.Fatal(err)
	}
	if err := db.FailOrRetryJob(ctx, job.ID, token, "invalid container", true); err != nil {
		t.Fatal(err)
	}
	// An email already retried seven times gets abandoned on the eighth.
	if _, err := db.DB.ExecContext(ctx, `UPDATE notification_email_outbox
		SET attempts=7, next_attempt_at=NOW()-INTERVAL '1 second'
		WHERE notification_id=(SELECT id FROM user_notifications WHERE job_id=$1::uuid)`, job.ID); err != nil {
		t.Fatal(err)
	}
	fail := &integrationMailer{err: errors.New("permanent relay rejection")}
	if count, err := db.DispatchFailureEmails(ctx, fail); err != nil || count != 0 {
		t.Fatalf("eighth email failure: %d %v", count, err)
	}
	var abandoned bool
	if err := db.DB.QueryRowContext(ctx, `SELECT abandoned_at IS NOT NULL FROM notification_email_outbox
		WHERE notification_id=(SELECT id FROM user_notifications WHERE job_id=$1::uuid)`, job.ID).Scan(&abandoned); err != nil || !abandoned {
		t.Fatalf("email was not abandoned: %v %v", abandoned, err)
	}
	if n, err := db.DispatchFailureEmails(ctx, &integrationMailer{}); err != nil || n != 0 {
		t.Fatalf("abandoned email retried: %d %v", n, err)
	}
}

func TestPostgresUnreachableAndClosedConnectionErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := Open(ctx, ""); err == nil {
		t.Fatal("missing DSN accepted")
	}
	// Local port 1 is unavailable on a normal runner and rejects immediately.
	if _, err := Open(ctx, "postgres://fiapx:invalid@127.0.0.1:1/fiapx?sslmode=disable"); err == nil {
		t.Fatal("unreachable database accepted")
	}
	sqlDB, err := sql.Open("postgres", "postgres://unused:unused@localhost:5432/unused?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	db := &Postgres{DB: sqlDB}
	for name, invoke := range map[string]func() error{
		"create user": func() error { return db.CreateUser(ctx, User{ID: testUUID(t), Email: "closed@example.test"}) },
		"email lookup": func() error { _, e := db.FindUserByEmail(ctx, "closed@example.test"); return e },
		"user lookup": func() error { _, e := db.FindUserByID(ctx, testUUID(t)); return e },
		"list jobs": func() error { _, e := db.ListJobsForUser(ctx, testUUID(t)); return e },
		"get job": func() error { _, e := db.GetJobForUser(ctx, testUUID(t), testUUID(t)); return e },
		"create job": func() error { _, e := db.CreateQueuedJob(ctx, testUUID(t), testUUID(t), "x.mp4", "x"); return e },
		"publish": func() error { _, e := db.PublishPending(ctx, &integrationPublisher{}); return e },
		"claim": func() error { _, _, e := db.ClaimJob(ctx, testUUID(t), testUUID(t)); return e },
		"renew": func() error { return db.RenewLease(ctx, testUUID(t), testUUID(t)) },
		"complete": func() error { return db.CompleteJob(ctx, testUUID(t), testUUID(t), "out.zip") },
		"fail": func() error { return db.FailOrRetryJob(ctx, testUUID(t), testUUID(t), "reason", true) },
		"download": func() error { _, e := db.GetCompletedOutput(ctx, testUUID(t), testUUID(t)); return e },
		"notifications": func() error { _, e := db.ListNotifications(ctx, testUUID(t)); return e },
		"read": func() error { return db.MarkNotificationRead(ctx, testUUID(t), 123) },
		"email outbox": func() error { _, e := db.DispatchFailureEmails(ctx, &integrationMailer{}); return e },
	} {
		if err := invoke(); err == nil {
			t.Errorf("%s: expected closed-database error", name)
		}
	}
}
