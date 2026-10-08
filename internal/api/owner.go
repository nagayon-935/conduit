package api

import (
	"net/http"
	"strings"

	"github.com/nagayon-935/conduit/internal/session"
)

// Session capabilities permit their holder to inspect/end only that session.
// Admin identifiers and read-only share tokens never grant owner access.
func (h *Handler) ownSession(w http.ResponseWriter, r *http.Request) (*session.Session, string) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")) == "" {
		apiError(w, http.StatusUnauthorized, "session token required", "UNAUTHORIZED")
		return nil, ""
	}
	token := strings.TrimPrefix(header, "Bearer ")
	sess, err := h.sessions.Get(token)
	if err != nil || sess.IsExpired() {
		apiError(w, http.StatusGone, "session ended or expired", "SESSION_ENDED")
		return nil, ""
	}
	return sess, token
}

func (h *Handler) handleOwnSession(w http.ResponseWriter, r *http.Request) {
	if sess, _ := h.ownSession(w, r); sess != nil {
		writeJSON(w, http.StatusOK, sess.Info())
	}
}

func (h *Handler) handleEndOwnSession(w http.ResponseWriter, r *http.Request) {
	if sess, token := h.ownSession(w, r); sess != nil {
		if err := h.sessions.TerminateWithReason(token, "利用者がセッションを終了しました。"); err != nil {
			apiError(w, http.StatusGone, "session ended", "SESSION_ENDED")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) handleOwnShares(w http.ResponseWriter, r *http.Request) {
	if sess, token := h.ownSession(w, r); sess != nil {
		writeJSON(w, http.StatusOK, h.sessions.ListShares(token))
	}
}

// Shared capabilities expose status only, never owner or admin operations.
func (h *Handler) handleSharedSession(w http.ResponseWriter, r *http.Request) {
	token, ok := h.sessions.ResolveShare(r.PathValue("shareToken"))
	if !ok {
		apiError(w, http.StatusGone, "share ended or expired", "SHARE_ENDED")
		return
	}
	sess, err := h.sessions.Get(token)
	if err != nil || sess.IsExpired() {
		apiError(w, http.StatusGone, "session ended or expired", "SHARE_ENDED")
		return
	}
	writeJSON(w, http.StatusOK, sess.Info())
}
