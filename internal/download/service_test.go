package download

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeJobs struct {
	key  string
	err  error
	user string
	id   string
}

func (f *fakeJobs) GetCompletedOutput(_ context.Context, u, id string) (string, error) {
	f.user, f.id = u, id
	return f.key, f.err
}

type fakeBlobs struct {
	key   string
	calls int
}

func (f *fakeBlobs) Get(_ context.Context, k string) (io.ReadCloser, error) {
	f.calls++
	f.key = k
	return io.NopCloser(strings.NewReader("zip")), nil
}
func TestDownloadChecksOwnershipFirst(t *testing.T) {
	jobs := &fakeJobs{err: sql.ErrNoRows}
	objects := &fakeBlobs{}
	service := New(jobs, objects)
	if _, err := service.OpenForUser(context.Background(), "other-user", "job"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ownership rejection, got %v", err)
	}
	if objects.calls != 0 {
		t.Fatal("read private object before checking owner")
	}
	jobs.err = nil
	jobs.key = "outputs/owner/job/frames.zip"
	body, err := service.OpenForUser(context.Background(), "owner", "job")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if objects.calls != 1 || objects.key != jobs.key {
		t.Fatal("incorrect object selected")
	}
}
