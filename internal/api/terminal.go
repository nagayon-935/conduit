package api

import (
	"crypto/rand"
	"fmt"
	"log/slog"

	"github.com/nagayon-935/conduit/internal/session"
)

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
