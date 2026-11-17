package httpapi

import (
	"net/http"
	"testing"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/auth"
)

func TestLoginAndRegistrationInvalidRequests(t *testing.T) {
	handler, _ := fixture(t)
	tests := []struct {
		name, route, body string
		code              int
	}{
		{"bad register JSON", "/api/auth/register", "{", http.StatusBadRequest},
		{"unknown registration field", "/api/auth/register", `{"email":"test@example.test","password":"correct-horse-battery-staple","admin":true}`, http.StatusBadRequest},
		{"extra JSON", "/api/auth/register", `{"email":"test@example.test","password":"correct-horse-battery-staple"} {}`, http.StatusBadRequest},
		{"empty login", "/api/auth/login", "not-json", http.StatusBadRequest},
		{"invalid login email", "/api/auth/login", `{"email":"invalid-address","password":"correct-horse-battery-staple"}`, http.StatusUnauthorized},
		{"missing login account", "/api/auth/login", `{"email":"missing@example.test","password":"correct-horse-battery-staple"}`, http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := request(t, handler, "POST", tc.route, tc.body, "")
			if response.Code != tc.code {
				t.Fatalf("unexpected response: HTTP %d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestJobRoutesValidateInputAndMissingUser(t *testing.T) {
	handler, _ := fixture(t)
	tokens, err := auth.NewTokens("a-local-secret-longer-than-thirty-two-bytes-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokens.Issue(userA)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/me", http.StatusUnauthorized},
		{"GET", "/api/jobs/not-a-uuid", http.StatusBadRequest},
		{"GET", "/api/jobs/" + jobA, http.StatusNotFound},
	} {
		res := request(t, handler, tc.method, tc.path, "", token)
		if res.Code != tc.status {
			t.Errorf("%s %s: HTTP %d, want %d: %s", tc.method, tc.path, res.Code, tc.status, res.Body.String())
		}
	}
}
