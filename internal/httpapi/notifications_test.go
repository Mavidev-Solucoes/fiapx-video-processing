package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/auth"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

type fakeNotices struct {
	owner        string
	notification store.Notification
	read         bool
}

func (f *fakeNotices) ListNotifications(_ context.Context, user string) ([]store.Notification, error) {
	if user != f.owner {
		return []store.Notification{}, nil
	}
	n := f.notification
	if f.read {
		t := time.Now()
		n.ReadAt = &t
	}
	return []store.Notification{n}, nil
}
func (f *fakeNotices) MarkNotificationRead(_ context.Context, user string, id int64) error {
	if user != f.owner || id != f.notification.ID {
		return sql.ErrNoRows
	}
	f.read = true
	return nil
}

func TestNotificationsRequireAuthenticationAndOwnership(t *testing.T) {
	db := &memoryStore{users: map[string]store.User{}, jobs: map[string][]store.Job{}}
	tokens, err := auth.NewTokens("a-local-secret-longer-than-thirty-two-bytes-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	accessA, _ := tokens.Issue(userA)
	accessB, _ := tokens.Issue(userB)
	notifications := &fakeNotices{
		owner: userA,
		notification: store.Notification{
			ID: 41, JobID: jobA, Category: "processing_failed",
			Message: "Vídeo não foi processado", CreatedAt: time.Now(),
		},
	}
	handler := NewWithNotifications(db, db, tokens, nil, nil, notifications)
	if response := request(t, handler, "GET", "/api/notifications", "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous read: %d", response.Code)
	}
	if response := request(t, handler, "GET", "/api/notifications", "", accessB); response.Code != http.StatusOK || response.Body.String() != "{\"notifications\":[]}\n" {
		t.Fatalf("other-user notifications: %d %s", response.Code, response.Body.String())
	}
	response := request(t, handler, "GET", "/api/notifications", "", accessA)
	if response.Code != http.StatusOK || !contains(response.Body.String(), jobA) {
		t.Fatalf("owner cannot see notification: %d %s", response.Code, response.Body.String())
	}
	path := "/api/notifications/41/read"
	if response := request(t, handler, "POST", path, "", accessB); response.Code != http.StatusNotFound || notifications.read {
		t.Fatalf("other-user modified notification: %d", response.Code)
	}
	if response := request(t, handler, "POST", path, "", accessA); response.Code != http.StatusNoContent || !notifications.read {
		t.Fatalf("owner could not acknowledge notification: %d", response.Code)
	}
	if response := request(t, handler, "POST", "/api/notifications/invalid/read", "", accessA); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid ID accepted: %d", response.Code)
	}
}
func contains(text, sub string) bool { return strings.Contains(text, sub) }
