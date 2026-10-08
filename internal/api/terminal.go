package api

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"time"

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

	var sess *session.Session
	var removedCh <-chan struct{}
	if readOnly {
		sess, removedCh, err = h.sessions.AttachShared(shareToken, connID, ws)
	} else {
		sess, removedCh, err = h.sessions.Attach(token, connID, ws, false)
	}
	if err != nil {
		slog.Warn("session attach failed", "token", token, "error", err)
		// Send exit (not error) so the client stops reconnecting.
		// A terminated/missing session is a permanent condition.
		sendWSExitReason(session.NewSafeConn(ws), "セッションが終了したか、再接続期限を過ぎています。")
		return
	}

	safeWS := sess.GetSafeConn(connID)
	if safeWS == nil {
		return
	}
	// Also expire an already connected viewer at the link's deadline.
	var shareExpiry *time.Timer
	if readOnly {
		for _, link := range h.sessions.ListShares(token) {
			if link.ShareToken == shareToken {
				shareExpiry = time.AfterFunc(time.Until(link.ExpiresAt), func() { h.sessions.RevokeShare(shareToken) })
				break
			}
		}
		if shareExpiry != nil {
			defer shareExpiry.Stop()
		}
	}
	_ = safeWS.WriteJSON(map[string]any{"type": "session", "session": sess.Info()})

	slog.Info("terminal connected", "token", token, "connID", connID)

	defer sess.RemoveWebSocket(connID)
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
		sendWSExitReason(safeWS, sess.Info().EndReason)
	case <-removedCh:
		// writePump already called RemoveWebSocket.
		// If the session also terminated (e.g. SSH exit), send exit so the
		// client doesn't attempt to reconnect.
		select {
		case <-sess.Done():
			sendWSExitReason(safeWS, sess.Info().EndReason)
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
	sendWSExitReason(ws, "セッションが終了しました。")
}

func sendWSExitReason(ws *session.SafeConn, reason string) {
	if ws == nil {
		return
	}
	type exitMsg struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
	}
	if err := ws.WriteJSON(exitMsg{Type: "exit", Reason: reason}); err != nil {
		slog.Warn("sendWSExit: write failed", "error", err)
	}
}
