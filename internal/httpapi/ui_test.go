package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestBrowserDashboardAndCSP(t *testing.T) {
	handler, _ := fixture(t)
	response := request(t, handler, "GET", "/", "", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "FIAP X") {
		t.Fatalf("dashboard unavailable: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("missing script CSP")
	}
	js := request(t, handler, "GET", "/app.js", "", "")
	if js.Code != http.StatusOK || !strings.Contains(js.Body.String(), "/api/notifications") {
		t.Fatalf("dashboard script unavailable: %d", js.Code)
	}
	css := request(t, handler, "GET", "/style.css", "", "")
	if css.Code != http.StatusOK || css.Header().Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatalf("styles unavailable: %d", css.Code)
	}
	if r := request(t, handler, "GET", "/not-a-real-page", "", ""); r.Code != http.StatusNotFound {
		t.Fatalf("unexpected route access: %d", r.Code)
	}
	if r := request(t, handler, "GET", "/api/this-does-not-exist", "", ""); r.Code != http.StatusNotFound {
		t.Fatalf("unexpected API route access: %d", r.Code)
	}
}
