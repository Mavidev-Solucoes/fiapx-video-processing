package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

type Notifications interface {
	ListNotifications(context.Context, string) ([]store.Notification, error)
	MarkNotificationRead(context.Context, string, int64) error
}

func (a *API) listNotifications(w http.ResponseWriter, r *http.Request, userID string) {
	if a.notifications == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications unavailable")
		return
	}
	notices, err := a.notifications.ListNotifications(r.Context(), userID)
	if err != nil { serverError(w, err); return }
	writeJSON(w, http.StatusOK, map[string]any{"notifications": notices})
}

func (a *API) markNotificationRead(w http.ResponseWriter, r *http.Request, userID string) {
	if a.notifications == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications unavailable")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid notification ID")
		return
	}
	err = a.notifications.MarkNotificationRead(r.Context(), userID, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	if err != nil { serverError(w, err); return }
	w.WriteHeader(http.StatusNoContent)
}
