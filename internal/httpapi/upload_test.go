package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/auth"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

type fakeUploader struct {
	count    int
	userID   string
	filename string
	body     string
	err      error
}

func (f *fakeUploader) Create(_ context.Context, userID, filename string, body io.Reader, size int64) (store.Job, error) {
	f.count++
	f.userID, f.filename = userID, filename
	payload, _ := io.ReadAll(body)
	f.body = string(payload)
	if f.err != nil {
		return store.Job{}, f.err
	}
	return store.Job{ID: jobA, Filename: filename, Status: "queued", CreatedAt: time.Now()}, nil
}

func uploadRequest(t *testing.T, h http.Handler, token, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("video", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthenticatedVideoUploadAccepted(t *testing.T) {
	db := &memoryStore{users: map[string]store.User{}, jobs: map[string][]store.Job{}}
	tokens, err := auth.NewTokens("a-local-secret-longer-than-thirty-two-bytes-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	access, err := tokens.Issue(userA)
	if err != nil {
		t.Fatal(err)
	}
	uploader := &fakeUploader{}
	handler := New(db, db, tokens, uploader)
	unauthorized := uploadRequest(t, handler, "", "movie.mp4", []byte("video"))
	if unauthorized.Code != http.StatusUnauthorized || uploader.count != 0 {
		t.Fatal("unauthorized upload was processed")
	}
	ok := uploadRequest(t, handler, access, "movie.mp4", []byte("video"))
	if ok.Code != http.StatusAccepted || ok.Header().Get("Location") != "/api/jobs/"+jobA {
		t.Fatalf("upload result: %d %s", ok.Code, ok.Body.String())
	}
	var result store.Job
	if err := json.Unmarshal(ok.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "queued" || uploader.userID != userA || uploader.body != "video" {
		t.Fatalf("invalid accepted upload: %+v, uploader %+v", result, uploader)
	}
}

func TestUploadRejectsAbsentVideo(t *testing.T) {
	db := &memoryStore{users: map[string]store.User{}, jobs: map[string][]store.Job{}}
	tokens, _ := auth.NewTokens("a-local-secret-longer-than-thirty-two-bytes-for-tests")
	access, _ := tokens.Issue(userA)
	uploader := &fakeUploader{err: errors.New("storage unavailable")}
	handler := New(db, db, tokens, uploader)
	empty := uploadRequest(t, handler, access, "empty.mp4", nil)
	if empty.Code != http.StatusBadRequest || uploader.count != 0 {
		t.Fatalf("empty upload unexpectedly accepted: %d", empty.Code)
	}
	failed := uploadRequest(t, handler, access, "movie.mp4", []byte("x"))
	if failed.Code != http.StatusInternalServerError || uploader.count != 1 {
		t.Fatalf("storage failure not surfaced: %d", failed.Code)
	}
}
