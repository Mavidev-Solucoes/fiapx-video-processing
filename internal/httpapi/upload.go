package httpapi

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/intake"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

type Uploader interface {
	Create(ctx context.Context, userID, originalName string, body io.Reader, size int64) (store.Job, error)
}

const multipartOverhead int64 = 2 << 20
const multipartMemory int64 = 8 << 20

func (a *API) uploadJob(w http.ResponseWriter, r *http.Request, userID string) {
	if a.uploader == nil {
		writeError(w, http.StatusServiceUnavailable, "upload service unavailable")
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		writeError(w, http.StatusUnsupportedMediaType, "multipart/form-data required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, intake.MaxVideoBytes+multipartOverhead)
	if err := r.ParseMultipartForm(multipartMemory); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "video exceeds 100 MiB limit")
		} else {
			writeError(w, http.StatusBadRequest, "invalid multipart upload")
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	headers := r.MultipartForm.File["video"]
	if len(headers) != 1 {
		writeError(w, http.StatusBadRequest, "exactly one video file is required")
		return
	}
	header := headers[0]
	if header.Size > intake.MaxVideoBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "video exceeds 100 MiB limit")
		return
	}
	if header.Size == 0 {
		writeError(w, http.StatusBadRequest, "video is empty")
		return
	}
	file, err := openVideo(header)
	if err != nil {
		serverError(w, err)
		return
	}
	defer file.Close()
	job, err := a.uploader.Create(r.Context(), userID, header.Filename, file, header.Size)
	if errors.Is(err, intake.ErrInvalidVideo) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported video format")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Location", "/api/jobs/"+job.ID)
	writeJSON(w, http.StatusAccepted, job)
}

func openVideo(header *multipart.FileHeader) (multipart.File, error) {
	return header.Open()
}
