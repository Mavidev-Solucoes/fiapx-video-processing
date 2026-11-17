package intake

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

type memoryObjects struct {
	key     string
	mime    string
	size    int64
	fail    bool
}

func (m *memoryObjects) Put(_ context.Context, key string, body io.Reader, size int64, mime string) error {
	if m.fail {
		return errors.New("object store unavailable")
	}
	content, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if int64(len(content)) != size {
		return errors.New("incorrect stored payload size")
	}
	m.key, m.mime, m.size = key, mime, size
	return nil
}

type memoryJobs struct {
	userID, jobID, filename, key string
	fail bool
}

func (m *memoryJobs) CreateQueuedJob(_ context.Context, userID, jobID, filename, key string) (store.Job, error) {
	if m.fail {
		return store.Job{}, errors.New("database unavailable")
	}
	m.userID, m.jobID, m.filename, m.key = userID, jobID, filename, key
	return store.Job{ID: jobID, Filename: filename, Status: "queued", CreatedAt: time.Now()}, nil
}

func TestCreateStoresVideoAndRecordsJob(t *testing.T) {
	objects, jobs := &memoryObjects{}, &memoryJobs{}
	service := New(objects, jobs)
	job, err := service.Create(context.Background(), "a-user", "../unsafe\\movie.MP4", strings.NewReader("video"), 5)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "queued" || jobs.userID != "a-user" || jobs.filename != "movie.MP4" {
		t.Fatalf("unexpected job %+v; filename %q", job, jobs.filename)
	}
	if objects.key != jobs.key || !strings.HasSuffix(objects.key, "/source.mp4") ||
		objects.mime != "video/mp4" || objects.size != 5 {
		t.Fatalf("unexpected object metadata: %+v", objects)
	}
}

func TestCreateRejectsUnsupportedAndEmptyVideo(t *testing.T) {
	objects, jobs := &memoryObjects{}, &memoryJobs{}
	service := New(objects, jobs)
	for _, tc := range []struct{ name string; size int64 }{
		{"script.exe", 2}, {"video.mp4", 0}, {"video.mp4", MaxVideoBytes + 1},
	} {
		if _, err := service.Create(context.Background(), "user", tc.name, strings.NewReader("x"), tc.size); !errors.Is(err, ErrInvalidVideo) {
			t.Fatalf("expected invalid video for %+v: %v", tc, err)
		}
	}
	if objects.key != "" || jobs.key != "" {
		t.Fatal("invalid video caused side effects")
	}
}

func TestCreateDoesNotRecordJobIfStorageFails(t *testing.T) {
	objects, jobs := &memoryObjects{fail: true}, &memoryJobs{}
	_, err := New(objects, jobs).Create(context.Background(), "user", "video.mp4", strings.NewReader("x"), 1)
	if err == nil || jobs.key != "" {
		t.Fatal("should not record job after failed storage")
	}
}
