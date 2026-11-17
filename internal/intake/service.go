package intake

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/identity"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

const MaxVideoBytes int64 = 100 << 20 // 100 MiB

var ErrInvalidVideo = errors.New("unsupported or empty video")

type Objects interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
}

type JobWriter interface {
	CreateQueuedJob(ctx context.Context, userID, jobID, filename, objectKey string) (store.Job, error)
}

type Service struct {
	objects Objects
	jobs    JobWriter
}

func New(objects Objects, jobs JobWriter) *Service {
	return &Service{objects: objects, jobs: jobs}
}

func (s *Service) Create(ctx context.Context, userID, originalName string, body io.Reader, size int64) (store.Job, error) {
	filename := path.Base(strings.ReplaceAll(originalName, "\\", "/"))
	ext := strings.ToLower(path.Ext(filename))
	if filename == "" || filename == "." || filename == "/" ||
		size <= 0 || size > MaxVideoBytes || !allowedExtension(ext) {
		return store.Job{}, ErrInvalidVideo
	}
	jobID, err := identity.NewUUID()
	if err != nil {
		return store.Job{}, err
	}
	key := fmt.Sprintf("inputs/%s/%s/source%s", userID, jobID, ext)
	if err := s.objects.Put(ctx, key, body, size, contentType(ext)); err != nil {
		return store.Job{}, err
	}
	// Store the video before accepting the job. Job and outbox entry commit
	// atomically. Retain the object on ambiguous SQL commit errors rather than
	// risk deleting data for a job that may have committed successfully.
	return s.jobs.CreateQueuedJob(ctx, userID, jobID, filename, key)
}

func allowedExtension(ext string) bool {
	switch ext {
	case ".mp4", ".mov", ".mkv", ".webm", ".avi":
		return true
	default:
		return false
	}
}

func contentType(ext string) string {
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	case ".avi":
		return "video/x-msvideo"
	default:
		return "application/octet-stream"
	}
}
