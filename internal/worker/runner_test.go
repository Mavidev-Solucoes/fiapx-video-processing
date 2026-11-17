package worker

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

type testJobs struct {
	state     store.ClaimState
	job       store.WorkJob
	completed string
	terminal  bool
	retried   bool
	mutations int
}

func (f *testJobs) ClaimJob(_ context.Context, id, token string) (store.WorkJob, store.ClaimState, error) {
	job := f.job
	job.ID, job.Token = id, token
	return job, f.state, nil
}
func (f *testJobs) RenewLease(context.Context, string, string) error { return nil }
func (f *testJobs) CompleteJob(_ context.Context, _, _, key string) error {
	f.completed = key
	f.mutations++
	f.state = store.Finished
	return nil
}
func (f *testJobs) FailOrRetryJob(_ context.Context, _, _, _ string, terminal bool) error {
	f.terminal, f.retried = terminal, !terminal
	f.mutations++
	if terminal {
		f.state = store.Finished
	}
	return nil
}

type testObjects struct {
	data   string
	output string
	getErr error
	putErr error
	puts   int
}

func (f *testObjects) Get(context.Context, string) (io.ReadCloser, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return io.NopCloser(strings.NewReader(f.data)), nil
}
func (f *testObjects) Put(_ context.Context, _ string, input io.Reader, _ int64, _ string) error {
	if f.putErr != nil {
		return f.putErr
	}
	data, err := io.ReadAll(input)
	if err != nil {
		return err
	}
	f.output = string(data)
	f.puts++
	return nil
}
func testFixture() (*Runner, *testJobs, *testObjects) {
	jobs := &testJobs{state: store.Claimed, job: store.WorkJob{
		ID:       "06ec2029-6d53-480b-8efb-6080bfb7c997",
		UserID:   "5a5c76d5-3f4b-4614-a6e9-375746b7f828",
		Filename: "sample.mp4", InputKey: "inputs/a/source.mp4", Attempt: 1,
	}}
	objects := &testObjects{data: "a video"}
	return New(jobs, objects), jobs, objects
}
func TestSuccessAndDuplicate(t *testing.T) {
	r, jobs, objects := testFixture()
	r.extract = func(_ context.Context, _, inputPath, outputPath string) (int, error) {
		data, err := os.ReadFile(inputPath)
		if err != nil || string(data) != "a video" {
			t.Fatalf("bad input: %s, %v", data, err)
		}
		if err := os.WriteFile(outputPath, []byte("ZIP"), 0600); err != nil {
			return 0, err
		}
		return 1, nil
	}
	ack, err := r.Handle(context.Background(), jobs.job.ID)
	if err != nil || !ack {
		t.Fatalf("expected terminal success: %t %v", ack, err)
	}
	if objects.output != "ZIP" || objects.puts != 1 || jobs.completed == "" || jobs.mutations != 1 {
		t.Fatalf("result was not persisted: %+v %+v", objects, jobs)
	}
	ack, err = r.Handle(context.Background(), jobs.job.ID)
	if err != nil || !ack || objects.puts != 1 || jobs.mutations != 1 {
		t.Fatalf("duplicate changed work: %t %v", ack, err)
	}
}
func TestInvalidVideoFailsWithoutRetry(t *testing.T) {
	r, jobs, _ := testFixture()
	r.extract = func(context.Context, string, string, string) (int, error) {
		return 0, errors.New("unrecognized media")
	}
	ack, err := r.Handle(context.Background(), jobs.job.ID)
	if err != nil || !ack || !jobs.terminal || jobs.retried {
		t.Fatalf("invalid video should fail and ack: %t %v", ack, err)
	}
}
func TestDependencyFailureRetriesThenTerminates(t *testing.T) {
	r, jobs, objects := testFixture()
	objects.getErr = errors.New("storage unavailable")
	ack, err := r.Handle(context.Background(), jobs.job.ID)
	if err != nil || ack || !jobs.retried || jobs.terminal {
		t.Fatalf("should retry: %t %v", ack, err)
	}
	jobs.job.Attempt = store.MaxAttempts
	ack, err = r.Handle(context.Background(), jobs.job.ID)
	if err != nil || !ack || !jobs.terminal {
		t.Fatalf("last attempt should fail: %t %v", ack, err)
	}
}
func TestBusyLeaseRequeues(t *testing.T) {
	r, jobs, objects := testFixture()
	jobs.state = store.Busy
	ack, err := r.Handle(context.Background(), jobs.job.ID)
	if err != nil || ack || objects.puts != 0 || jobs.mutations != 0 {
		t.Fatalf("busy job should requeue: %t %v", ack, err)
	}
}
