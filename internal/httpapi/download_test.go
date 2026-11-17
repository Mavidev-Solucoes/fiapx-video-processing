package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/auth"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

type fakeDownload struct {
	expectedUser string
	expectedJob  string
	data         string
	err          error
	calls        int
}

func (f *fakeDownload) OpenForUser(_ context.Context, userID, jobID string) (io.ReadCloser, error) {
	f.calls++
	if userID != f.expectedUser || jobID != f.expectedJob {
		return nil, sql.ErrNoRows
	}
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.data)), nil
}

func TestDownloadOwnershipAndStatuses(t *testing.T) {
	db := &memoryStore{users: map[string]store.User{}, jobs: map[string][]store.Job{}}
	tokenService, err := auth.NewTokens("a-local-secret-longer-than-thirty-two-bytes-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	tokenA, _ := tokenService.Issue(userA)
	tokenB, _ := tokenService.Issue(userB)
	download := &fakeDownload{expectedUser: userA, expectedJob: jobA, data: "ZIP-CONTENT"}
	handler := NewWithDownload(db, db, tokenService, nil, download)
	path := "/api/jobs/" + jobA + "/download"
	if r := request(t, handler, "GET", path, "", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous download: %d", r.Code)
	}
	if r := request(t, handler, "GET", path, "", tokenB); r.Code != http.StatusNotFound {
		t.Fatalf("cross-user download: %d", r.Code)
	}
	r := request(t, handler, "GET", path, "", tokenA)
	if r.Code != http.StatusOK || r.Body.String() != "ZIP-CONTENT" ||
		r.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("download: %d %s", r.Code, r.Body.String())
	}
	download.err = store.ErrNotReady
	if r := request(t, handler, "GET", path, "", tokenA); r.Code != http.StatusConflict {
		t.Fatalf("pending download: %d", r.Code)
	}
	download.err = errors.New("object storage offline")
	if r := request(t, handler, "GET", path, "", tokenA); r.Code != http.StatusInternalServerError {
		t.Fatalf("storage outage: %d", r.Code)
	}
	if r := request(t, handler, "GET", "/api/jobs/bad/download", "", tokenA); r.Code != http.StatusBadRequest {
		t.Fatalf("invalid job id: %d", r.Code)
	}
}
