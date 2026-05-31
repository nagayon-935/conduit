package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// handleGetRecording serves the .cast file for a given connection log ID.
// It resolves the path via the log store (not directly from the URL) to prevent
// path traversal.
func (h *Handler) handleGetRecording(w http.ResponseWriter, r *http.Request) {
	logID := r.PathValue("id")
	if logID == "" {
		apiError(w, http.StatusBadRequest, "id is required", "BAD_REQUEST")
		return
	}

	// Resolve the recording path via the log store (indirect lookup = no traversal).
	var castPath string
	for _, e := range h.logs.List() {
		if e.ID == logID {
			castPath = e.RecordingPath
			break
		}
	}
	if castPath == "" {
		apiError(w, http.StatusNotFound, "recording not found", "NOT_FOUND")
		return
	}

	// Extra safety: confirm the resolved path is inside RecordingDir.
	absDir, err := filepath.Abs(h.config.RecordingDir)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "internal error", "INTERNAL_ERROR")
		return
	}
	absPath, err := filepath.Abs(castPath)
	if err != nil || !strings.HasPrefix(absPath, absDir+string(os.PathSeparator)) {
		apiError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	w.Header().Set("Content-Type", "application/x-asciicast")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, absPath)
}
