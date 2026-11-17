package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/identity"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

type Downloader interface {
	OpenForUser(context.Context, string, string) (io.ReadCloser, error)
}

func (a *API) downloadJob(w http.ResponseWriter, r *http.Request, userID string) {
	if a.downloader == nil {
		writeError(w, http.StatusServiceUnavailable, "downloads unavailable")
		return
	}
	jobID := r.PathValue("id")
	if !identity.ValidUUID(jobID) {
		writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}
	content, err := a.downloader.OpenForUser(r.Context(), userID, jobID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if errors.Is(err, store.ErrNotReady) {
		writeError(w, http.StatusConflict, "video is not completed")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	defer content.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="fiapx-`+jobID+`.zip"`)
	w.Header().Set("Cache-Control", "private, no-store")
	if _, err := io.Copy(w, content); err != nil {
		log.Printf("download job %s stream interrupted: %v", jobID, err)
	}
}
