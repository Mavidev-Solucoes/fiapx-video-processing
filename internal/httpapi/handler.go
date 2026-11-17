package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/mail"
	"strings"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/auth"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/identity"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

type API struct {
	users  store.Users
	jobs   store.Jobs
	tokens *auth.Tokens
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func New(users store.Users, jobs store.Jobs, tokens *auth.Tokens) http.Handler {
	a := &API{users: users, jobs: jobs, tokens: tokens}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"service": "fiapx-api", "status": "alive"})
	})
	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("GET /api/me", a.protected(a.me))
	mux.HandleFunc("GET /api/jobs", a.protected(a.listJobs))
	mux.HandleFunc("GET /api/jobs/{id}", a.protected(a.getJob))
	return mux
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := readJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email, ok := normalizeEmail(input.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "password must contain between 12 and 72 bytes")
		return
	}
	id, err := identity.NewUUID()
	if err != nil {
		serverError(w, err)
		return
	}
	if err := a.users.CreateUser(r.Context(), store.User{ID: id, Email: email, PasswordHash: hash}); err != nil {
		if errors.Is(err, store.ErrEmailExists) {
			writeError(w, http.StatusConflict, "email is already registered")
			return
		}
		serverError(w, err)
		return
	}
	token, err := a.tokens.Issue(id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":        map[string]string{"id": id, "email": email},
		"accessToken": token, "tokenType": "Bearer", "expiresInSeconds": 3600,
	})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := readJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email, ok := normalizeEmail(input.Email)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	user, err := a.users.FindUserByEmail(r.Context(), email)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if !auth.CheckPassword(user.PasswordHash, input.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := a.tokens.Issue(user.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":        map[string]string{"id": user.ID, "email": user.Email},
		"accessToken": token, "tokenType": "Bearer", "expiresInSeconds": 3600,
	})
}

func (a *API) protected(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(w, http.StatusUnauthorized, "*** required")
			return
		}
		userID, err := a.tokens.Verify(parts[1])
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired access token")
			return
		}
		next(w, r, userID)
	}
}

func (a *API) me(w http.ResponseWriter, r *http.Request, userID string) {
	user, err := a.users.FindUserByID(r.Context(), userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": user.ID, "email": user.Email})
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request, userID string) {
	jobs, err := a.jobs.ListJobsForUser(r.Context(), userID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (a *API) getJob(w http.ResponseWriter, r *http.Request, userID string) {
	id := r.PathValue("id")
	if !identity.ValidUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}
	job, err := a.jobs.GetJobForUser(r.Context(), userID, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func normalizeEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if len(email) == 0 || len(email) > 254 {
		return "", false
	}
	address, err := mail.ParseAddress(email)
	return email, err == nil && address.Address == email && strings.Contains(email, "@")
}

func readJSON(w http.ResponseWriter, r *http.Request, value any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return errors.New("content-type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("unexpected data after JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("request failed: %v", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}
