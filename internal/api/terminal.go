package api

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/nagayon-935/conduit/internal/session"
	"github.com/nagayon-935/conduit/internal/tunnel"
)

// handleTerminal implements GET /ws?token=<session_token> (read-write)
// and GET /ws?share=<share_token> (read-only viewer).
func (h *Handler) handleTerminal(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	token := q.Get("token")
	shareToken := q.Get("share")
	readOnly := false

	if token == "" && shareToken == "" {
		apiError(w, http.StatusBadRequest, "token or share query parameter is required", "MISSING_TOKEN")
		return
	}

	if shareToken != "" {
		sessionToken, ok := h.sessions.ResolveShare(shareToken)
		if !ok {
			apiError(w, http.StatusForbidden, "share token is invalid or expired", "INVALID_SHARE_TOKEN")
			return
		}
		token = sessionToken
		readOnly = true
	}

	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer ws.Close()

	connID := generateConnID()

	sess, removedCh, err := h.sessions.Attach(token, connID, ws, readOnly)
	if err != nil {
		slog.Warn("session attach failed", "token", token, "error", err)
		// Send exit (not error) so the client stops reconnecting.
		// A terminated/missing session is a permanent condition.
		sendWSExit(session.NewSafeConn(ws))
		return
	}

	safeWS := sess.GetSafeConn(connID)

	slog.Info("terminal connected", "token", token, "connID", connID)

	cfg := tunnel.DefaultPumpConfig()

	// Start session-level pumps exactly once (shared across all tabs).
	sess.StartOnce(func() {
		tunnel.StartSessionPumps(sess.Context(), sess, cfg)
	})

	// Start per-connection write pump for this WebSocket.
	tunnel.StartConnectionPump(connID, ws, sess, cfg)

	// Block until this connection closes or the session terminates.
	select {
	case <-sess.Done():
		sendWSExit(safeWS)
	case <-removedCh:
		// writePump already called RemoveWebSocket.
		// If the session also terminated (e.g. SSH exit), send exit so the
		// client doesn't attempt to reconnect.
		select {
		case <-sess.Done():
			sendWSExit(safeWS)
		default:
		}
	}

	slog.Info("terminal disconnected", "token", token, "connID", connID)
}

func generateConnID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func sendWSExit(ws *session.SafeConn) {
	if ws == nil {
		return
	}
	type exitMsg struct {
		Type string `json:"type"`
	}
	if err := ws.WriteJSON(exitMsg{Type: "exit"}); err != nil {
		slog.Warn("sendWSExit: write failed", "error", err)
	}
}

