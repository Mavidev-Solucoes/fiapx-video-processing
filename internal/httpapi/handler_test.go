package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/auth"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

const (
	userA = "b69bcc39-98fc-4308-8908-a79d9c708457"
	userB = "cf85b4ac-af55-499b-b4b4-1b547fc64d61"
	jobA  = "ea8e33a2-7d9e-4e88-8829-9a74a79a8499"
)

type memoryStore struct {
	mu    sync.Mutex
	users map[string]store.User
	jobs  map[string][]store.Job
}

func (m *memoryStore) CreateUser(_ context.Context, u store.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.Email]; ok {
		return store.ErrEmailExists
	}
	m.users[u.Email] = u
	return nil
}

func (m *memoryStore) FindUserByEmail(_ context.Context, email string) (store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[email]
	if !ok {
		return store.User{}, sql.ErrNoRows
	}
	return u, nil
}

func (m *memoryStore) FindUserByID(_ context.Context, id string) (store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return store.User{}, sql.ErrNoRows
}

func (m *memoryStore) ListJobsForUser(_ context.Context, user string) ([]store.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]store.Job, len(m.jobs[user]))
	copy(result, m.jobs[user])
	return result, nil
}

func (m *memoryStore) GetJobForUser(_ context.Context, user, id string) (store.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs[user] {
		if j.ID == id {
			return j, nil
		}
	}
	return store.Job{}, sql.ErrNoRows
}

func fixture(t *testing.T) (http.Handler, *memoryStore) {
	t.Helper()
	m := &memoryStore{users: make(map[string]store.User), jobs: make(map[string][]store.Job)}
	tokens, err := auth.NewTokens("a-local-secret-longer-than-thirty-two-bytes-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	return New(m, m, tokens), m
}

func request(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)
	return recorder
}

func register(t *testing.T, handler http.Handler, email string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": "correct-horse-battery-staple"})
	response := request(t, handler, "POST", "/api/auth/register", string(body), "")
	if response.Code != http.StatusCreated {
		t.Fatalf("register %s: %d %s", email, response.Code, response.Body.String())
	}
	var data struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	return data.AccessToken
}

func TestRegisterLoginAndProtectedProfile(t *testing.T) {
	handler, _ := fixture(t)
	token := register(t, handler, "  Example@Email.com ")
	if request(t, handler, "GET", "/api/me", "", "").Code != http.StatusUnauthorized {
		t.Fatal("anonymous request should not access profile")
	}
	profile := request(t, handler, "GET", "/api/me", "", token)
	if profile.Code != http.StatusOK || !bytes.Contains(profile.Body.Bytes(), []byte("example@email.com")) {
		t.Fatalf("unexpected profile: %d %s", profile.Code, profile.Body.String())
	}
	bad := request(t, handler, "POST", "/api/auth/login", `{"email":"example@email.com","password":"wrong-password"}`, "")
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password accepted: %d", bad.Code)
	}
	ok := request(t, handler, "POST", "/api/auth/login", `{"email":"example@email.com","password":"correct-horse-battery-staple"}`, "")
	if ok.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", ok.Code, ok.Body.String())
	}
	duplicate := request(t, handler, "POST", "/api/auth/register", `{"email":"EXAMPLE@EMAIL.COM","password":"correct-horse-battery-staple"}`, "")
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate accepted: %d", duplicate.Code)
	}
}

func TestJobsAreScopedToAuthenticatedUser(t *testing.T) {
	handler, db := fixture(t)
	tokens, err := auth.NewTokens("a-local-secret-longer-than-thirty-two-bytes-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := tokens.Issue(userA)
	second, _ := tokens.Issue(userB)
	db.jobs[userA] = []store.Job{{
		ID: jobA, Filename: "a.mp4", Status: "completed", CreatedAt: time.Now(),
	}}
	other := request(t, handler, "GET", "/api/jobs/"+jobA, "", second)
	if other.Code != http.StatusNotFound {
		t.Fatalf("user B can read user A's job: %d", other.Code)
	}
	listB := request(t, handler, "GET", "/api/jobs", "", second)
	if listB.Code != http.StatusOK || bytes.Contains(listB.Body.Bytes(), []byte(jobA)) {
		t.Fatalf("user B can list user A's job: %d %s", listB.Code, listB.Body.String())
	}
	found := request(t, handler, "GET", "/api/jobs/"+jobA, "", first)
	if found.Code != http.StatusOK || !bytes.Contains(found.Body.Bytes(), []byte(jobA)) {
		t.Fatalf("user A cannot read own job: %d %s", found.Code, found.Body.String())
	}
	if request(t, handler, "GET", "/api/jobs", "", "").Code != http.StatusUnauthorized {
		t.Fatal("anonymous jobs listing should be rejected")
	}
}

func TestRegistrationValidatesInput(t *testing.T) {
	handler, _ := fixture(t)
	if request(t, handler, "POST", "/api/auth/register", `{"email":"not-an-email","password":"correct-horse-battery-staple"}`, "").Code != http.StatusBadRequest {
		t.Fatal("invalid email accepted")
	}
	if request(t, handler, "POST", "/api/auth/register", `{"email":"example@email.com","password":"short"}`, "").Code != http.StatusBadRequest {
		t.Fatal("short password accepted")
	}
}
