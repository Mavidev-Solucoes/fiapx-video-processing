package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/identity"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/processing"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

var ErrInvalidMedia = errors.New("video cannot be decoded or contains no frames")

type Jobs interface {
	ClaimJob(context.Context, string, string) (store.WorkJob, store.ClaimState, error)
	RenewLease(context.Context, string, string) error
	CompleteJob(context.Context, string, string, string) error
	FailOrRetryJob(context.Context, string, string, string, bool) error
}

type Objects interface {
	Get(context.Context, string) (io.ReadCloser, error)
	Put(context.Context, string, io.Reader, int64, string) error
}

type Extractor func(context.Context, string, string, string) (int, error)

type Runner struct {
	jobs    Jobs
	objects Objects
	extract Extractor
}

func New(jobs Jobs, objects Objects) *Runner {
	return &Runner{jobs: jobs, objects: objects, extract: processing.ExtractToZip}
}

// Handle returns true only after a terminal database state has been committed
// (or a previously completed/failed job is detected). A false result requires
// RabbitMQ redelivery, including when another worker currently owns the lease.
func (r *Runner) Handle(ctx context.Context, jobID string) (bool, error) {
	token, err := identity.NewUUID()
	if err != nil {
		return false, err
	}
	job, status, err := r.jobs.ClaimJob(ctx, jobID, token)
	if err != nil {
		return false, err
	}
	switch status {
	case store.Finished, store.Missing:
		return true, nil
	case store.Busy:
		return false, nil
	case store.Claimed:
	default:
		return false, fmt.Errorf("unknown claim state %q", status)
	}
	if job.Attempt > store.MaxAttempts {
		if err := r.jobs.FailOrRetryJob(ctx, job.ID, token,
			"Processamento interrompido repetidamente", true); err != nil {
			return false, err
		}
		return true, nil
	}
	// A running job renews its lease. If a heartbeat fails, cancel FFmpeg
	// and do not finalize a result with a potentially stale lease token.
	workCtx, cancel := context.WithCancel(ctx)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				return
			case <-ticker.C:
				hbCtx, stop := context.WithTimeout(workCtx, 10*time.Second)
				err := r.jobs.RenewLease(hbCtx, job.ID, token)
				stop()
				if err != nil {
					log.Printf("job %s lease renewal failed: %v", job.ID, err)
					cancel()
					return
				}
			}
		}
	}()
	key, processErr := r.process(workCtx, job)
	leaseInterrupted := workCtx.Err() != nil
	cancel()
	<-heartbeatDone
	if leaseInterrupted || ctx.Err() != nil {
		return false, fmt.Errorf("processing interrupted before completion")
	}
	if processErr == nil {
		if err := r.jobs.CompleteJob(ctx, job.ID, token, key); err != nil {
			return false, err
		}
		log.Printf("job %s completed on attempt %d", job.ID, job.Attempt)
		return true, nil
	}
	terminal := errors.Is(processErr, ErrInvalidMedia) || job.Attempt >= store.MaxAttempts
	reason := "Falha temporária no processamento; nova tentativa pendente"
	if terminal {
		reason = "Não foi possível processar o vídeo"
	}
	if err := r.jobs.FailOrRetryJob(ctx, job.ID, token, reason, terminal); err != nil {
		return false, err
	}
	log.Printf("job %s attempt %d failed (terminal=%t): %v", job.ID, job.Attempt, terminal, processErr)
	return terminal, nil
}

func (r *Runner) process(ctx context.Context, job store.WorkJob) (string, error) {
	tempDir, err := os.MkdirTemp("", "fiapx-work-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)
	ext := filepath.Ext(job.Filename)
	inputPath := filepath.Join(tempDir, "input"+ext)
	outputPath := filepath.Join(tempDir, "frames.zip")

	source, err := r.objects.Get(ctx, job.InputKey)
	if err != nil {
		return "", err
	}
	input, err := os.Create(inputPath)
	if err != nil {
		_ = source.Close()
		return "", err
	}
	_, copyErr := io.Copy(input, source)
	closeErr := input.Close()
	sourceErr := source.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if sourceErr != nil {
		return "", sourceErr
	}
	if _, err := r.extract(ctx, "", inputPath, outputPath); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%w: %v", ErrInvalidMedia, err)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return "", err
	}
	archive, err := os.Open(outputPath)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	key := fmt.Sprintf("outputs/%s/%s/frames.zip", job.UserID, job.ID)
	if err := r.objects.Put(ctx, key, archive, info.Size(), "application/zip"); err != nil {
		return "", err
	}
	return key, nil
}
