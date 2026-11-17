package monitoring

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeSource struct {
	result Snapshot
	err    error
}

func (f fakeSource) ReadSnapshot(context.Context) (Snapshot, error) {
	return f.result, f.err
}

func TestMetricsEndpointExposesOnlyAggregateData(t *testing.T) {
	handler := Handler(fakeSource{result: Snapshot{
		Queued: 3, Processing: 2, Completed: 8, Failed: 1,
		OutboxPending: 4, UnreadNotifications: 1, Attempts: 15,
		OldestQueuedSeconds: 42.5, OldestProcessingSeconds: 13.25,
	}})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("metrics returned HTTP %d", w.Code)
	}
	body := w.Body.String()
	for _, expected := range []string{
		`fiapx_jobs{status="queued"} 3`,
		`fiapx_jobs{status="completed"} 8`,
		"fiapx_outbox_pending 4",
		"fiapx_notifications_unread 1",
		"fiapx_oldest_queued_age_seconds 42.500",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metric %q missing from:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "userId") || strings.Contains(body, "input_object_key") {
		t.Fatalf("metrics leaked private job fields")
	}
}

func TestMetricsDependencyFailureIsNotReportedAsHealthy(t *testing.T) {
	handler := Handler(fakeSource{err: errors.New("database unavailable")})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable data returned HTTP %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "database unavailable") {
		t.Fatal("internal database information was leaked")
	}
}

func TestMetricsRejectsOtherRoutesAndMethods(t *testing.T) {
	handler := Handler(fakeSource{})
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{http.MethodGet, "/admin", http.StatusNotFound},
		{http.MethodPost, "/metrics", http.StatusMethodNotAllowed},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.code {
			t.Errorf("%s %s returned %d; want %d", tc.method, tc.path, w.Code, tc.code)
		}
	}
}
