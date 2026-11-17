package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var ErrEmailExists = errors.New("email is already registered")

type User struct {
	ID           string
	Email        string
	PasswordHash string
}

type Job struct {
	ID           string     `json:"id"`
	Filename     string     `json:"filename"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
}

type Users interface {
	CreateUser(context.Context, User) error
	FindUserByEmail(context.Context, string) (User, error)
	FindUserByID(context.Context, string) (User, error)
}

type Jobs interface {
	ListJobsForUser(context.Context, string) ([]Job, error)
	GetJobForUser(context.Context, string, string) (Job, error)
}

type Postgres struct {
	DB *sql.DB
}

func Open(ctx context.Context, dsn string) (*Postgres, error) {
	if dsn == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	return &Postgres{DB: db}, nil
}

func (p *Postgres) CreateUser(ctx context.Context, user User) error {
	_, err := p.DB.ExecContext(ctx,
		"INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)",
		user.ID, user.Email, user.PasswordHash)
	if err != nil {
		var pgError *pq.Error
		if errors.As(err, &pgError) && pgError.Code == "23505" {
			return ErrEmailExists
		}
	}
	return err
}

func (p *Postgres) FindUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := p.DB.QueryRowContext(ctx,
		"SELECT id::text, email, password_hash FROM users WHERE email = $1",
		email).Scan(&user.ID, &user.Email, &user.PasswordHash)
	return user, err
}

func (p *Postgres) FindUserByID(ctx context.Context, id string) (User, error) {
	var user User
	err := p.DB.QueryRowContext(ctx,
		"SELECT id::text, email, password_hash FROM users WHERE id = $1",
		id).Scan(&user.ID, &user.Email, &user.PasswordHash)
	return user, err
}

func (p *Postgres) ListJobsForUser(ctx context.Context, userID string) ([]Job, error) {
	rows, err := p.DB.QueryContext(ctx,
		"SELECT id::text, original_filename, status, COALESCE(error_message, ''), created_at, started_at, completed_at FROM video_jobs WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT 100",
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

func (p *Postgres) GetJobForUser(ctx context.Context, userID, jobID string) (Job, error) {
	row := p.DB.QueryRowContext(ctx,
		"SELECT id::text, original_filename, status, COALESCE(error_message, ''), created_at, started_at, completed_at FROM video_jobs WHERE user_id = $1 AND id = $2",
		userID, jobID)
	return scanJob(row)
}

type scanner interface {
	Scan(...any) error
}

func scanJob(row scanner) (Job, error) {
	var job Job
	var started, completed sql.NullTime
	err := row.Scan(&job.ID, &job.Filename, &job.Status,
		&job.ErrorMessage, &job.CreatedAt, &started, &completed)
	if err != nil {
		return Job{}, err
	}
	if started.Valid {
		job.StartedAt = &started.Time
	}
	if completed.Valid {
		job.CompletedAt = &completed.Time
	}
	return job, nil
}
