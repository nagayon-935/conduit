package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type shareResponse struct {
	ShareToken string `json:"share_token"`
	URL        string `json:"url"`
	ExpiresAt  string `json:"expires_at"`
}

// handleCreateShare implements POST /api/sessions/{token}/share.
// It issues a read-only share token for the given session.
func (h *Handler) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	sessionToken := r.PathValue("token")
	if sessionToken == "" {
		apiError(w, http.StatusBadRequest, "token is required", "BAD_REQUEST")
		return
	}

	var options struct {
		TTLSeconds int64 `json:"ttl_seconds"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&options); err != nil && err != io.EOF {
			apiError(w, http.StatusBadRequest, "invalid share options", "BAD_REQUEST")
			return
		}
	}
	if options.TTLSeconds == 0 {
		options.TTLSeconds = 4 * 60 * 60
	}
	if options.TTLSeconds < 1 || options.TTLSeconds > 4*60*60 {
		apiError(w, http.StatusBadRequest, "share duration must be between 1s and 4h", "BAD_REQUEST")
		return
	}
	shareToken, expiresAt, err := h.sessions.ShareFor(sessionToken, time.Duration(options.TTLSeconds)*time.Second)
	if err != nil {
		apiError(w, http.StatusNotFound, "session not found or terminated", "NOT_FOUND")
		return
	}

	// Relative URLs preserve the browser's HTTPS origin behind a reverse proxy.
	viewerURL := "/app?share=" + shareToken

	writeJSON(w, http.StatusCreated, shareResponse{
		ShareToken: shareToken,
		URL:        viewerURL,
		ExpiresAt:  expiresAt.UTC().Format(timeFormatUTC),
	})
}

// handleRevokeShare implements DELETE /api/sessions/{token}/share/{shareToken}.
func (h *Handler) handleRevokeShare(w http.ResponseWriter, r *http.Request) {
	shareToken := r.PathValue("shareToken")
	if shareToken == "" {
		apiError(w, http.StatusBadRequest, "shareToken is required", "BAD_REQUEST")
		return
	}
	if _, err := h.sessions.Get(r.PathValue("token")); err != nil {
		apiError(w, http.StatusNotFound, "session not found", "NOT_FOUND")
		return
	}
	if err := h.sessions.RevokeSessionShare(r.PathValue("token"), shareToken); err != nil {
		apiError(w, http.StatusNotFound, "share link not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
