package api

import (
	"fmt"
	"net/http"
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

	shareToken, expiresAt, err := h.sessions.Share(sessionToken)
	if err != nil {
		apiError(w, http.StatusNotFound, "session not found or terminated", "NOT_FOUND")
		return
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// Build the viewer URL. The frontend reads ?share=<token> on mount.
	viewerURL := fmt.Sprintf("%s://%s/?share=%s", scheme, r.Host, shareToken)

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
	h.sessions.RevokeShare(shareToken)
	w.WriteHeader(http.StatusNoContent)
}
