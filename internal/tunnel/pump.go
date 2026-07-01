package tunnel

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/session"
)

type PumpConfig struct {
	WriteTimeout        time.Duration
	BackpressureTimeout time.Duration
}

func DefaultPumpConfig() PumpConfig {
	return PumpConfig{
		WriteTimeout:        10 * time.Second,
		BackpressureTimeout: 50 * time.Millisecond,
	}
}

type wsMessage struct {
	Type    string `json:"type"`
	Cols    uint32 `json:"cols,omitempty"`
	Rows    uint32 `json:"rows,omitempty"`
	Message string `json:"message,omitempty"`
}

// StartSessionPumps launches goroutines that live for the entire session lifetime:
//   - sshToClientPump: SSH stdout → sess.ToClient
//   - readPump: sess.ToClient → broadcast to all WebSockets
//   - StartStdinForwarder: sess.FromClient → SSH stdin
//
// Must be called exactly once per session (enforced by sess.StartOnce at call site).
func StartSessionPumps(ctx context.Context, sess *session.Session, cfg PumpConfig) {
	go sshToClientPump(ctx, sess, cfg)
	go readPump(ctx, sess, cfg)
	StartStdinForwarder(ctx, sess)
}

// StartConnectionPump launches the writePump for a single WebSocket connection.
// Must be called once per WebSocket connection from handleTerminal.
func StartConnectionPump(connID string, ws *websocket.Conn, sess *session.Session, cfg PumpConfig) {
	safeWS := sess.GetSafeConn(connID)
	readOnly := sess.IsReadOnly(connID)
	go writePump(connID, ws, safeWS, sess, cfg, readOnly)
}

const sshReadBufSize = 32 * 1024 // 32 KB

// sshToClientPump reads SSH stdout and pushes bytes into sess.ToClient.
func sshToClientPump(ctx context.Context, sess *session.Session, cfg PumpConfig) {
	buf := make([]byte, sshReadBufSize)
	for {
		n, err := sess.Stdout.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			if sess.Recorder != nil {
				sess.Recorder.WriteOutput(data)
			}
			DrainOrDrop(sess.ToClient, data, cfg.BackpressureTimeout)
		}
		if err != nil {
			if err != io.EOF {
				slog.Error("sshToClientPump: read error", "error", err)
				sess.CloseWithError(err)
			} else {
				sess.Close()
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-sess.Done():
			return
		default:
		}
	}
}

// readPump reads from sess.ToClient and broadcasts to all connected WebSockets.
// This is a session-level goroutine – runs until the session terminates.
func readPump(ctx context.Context, sess *session.Session, cfg PumpConfig) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-sess.Done():
			return
		case data, ok := <-sess.ToClient:
			if !ok {
				return
			}
			sess.BroadcastToWebSockets(websocket.BinaryMessage, data)
		}
	}
}

// writePump reads WebSocket messages and routes them for a single connection.
// This is a connection-level goroutine – exits when the WebSocket closes.
// It calls sess.RemoveWebSocket(connID) on exit via defer.
// ws is used for reads; safeWS is used for writes (serialized via mutex).
// readOnly=true: binary frames (stdin) and non-ping control frames are silently
// discarded so a viewer cannot write to the SSH process.
func writePump(connID string, ws *websocket.Conn, safeWS *session.SafeConn, sess *session.Session, cfg PumpConfig, readOnly bool) {
	defer sess.RemoveWebSocket(connID)
	for {
		select {
		case <-sess.Done():
			return
		default:
		}

		msgType, msg, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("writePump: WebSocket closed unexpectedly", "error", err)
			}
			return
		}

		if msgType == websocket.TextMessage {
			handleControlMessage(safeWS, sess, msg, cfg, readOnly)
			continue
		}
		if readOnly {
			// Binary frame = raw stdin — silently discard for viewer connections.
			continue
		}
		DrainOrDrop(sess.FromClient, msg, cfg.BackpressureTimeout)
	}
}

// handleControlMessage parses a JSON control frame and acts on it.
// ws is the specific connection that sent the frame (for ping→pong responses).
// readOnly=true: resize and raw-input fallback are discarded (viewers cannot
// affect the SSH process or resize the PTY).
func handleControlMessage(ws *session.SafeConn, sess *session.Session, msg []byte, cfg PumpConfig, readOnly bool) {
	var frame wsMessage
	if err := json.Unmarshal(msg, &frame); err != nil {
		if !readOnly {
			DrainOrDrop(sess.FromClient, msg, cfg.BackpressureTimeout)
		}
		return
	}

	switch frame.Type {
	case "ping":
		pong, _ := json.Marshal(wsMessage{Type: "pong"})
		if err := ws.WriteWithDeadline(time.Now().Add(cfg.WriteTimeout), websocket.TextMessage, pong); err != nil {
			slog.Warn("handleControlMessage: pong write error", "error", err)
		}
	case "resize":
		if readOnly {
			// Viewers must not resize the PTY — that affects all connected users.
			return
		}
		size := WindowSize{Cols: frame.Cols, Rows: frame.Rows}
		if err := ResizePTY(sess.SSHSession, size); err != nil {
			slog.Warn("handleControlMessage: resize PTY error", "error", err)
		}
		if sess.Recorder != nil {
			sess.Recorder.WriteResize(frame.Cols, frame.Rows)
		}
	default:
		if readOnly {
			return
		}
		// Unknown or unrecognised JSON frame — treat as raw terminal input.
		// This handles the edge case where the user's input happens to be
		// valid JSON (e.g. "null", "{}", …) so it shouldn't be silently dropped.
		DrainOrDrop(sess.FromClient, msg, cfg.BackpressureTimeout)
	}
}

// StartStdinForwarder reads from sess.FromClient and writes to SSH stdin.
func StartStdinForwarder(ctx context.Context, sess *session.Session) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sess.Done():
				return
			case data, ok := <-sess.FromClient:
				if !ok {
					return
				}
				if _, err := sess.Stdin.Write(data); err != nil {
					slog.Error("stdinForwarder: write to SSH stdin failed", "error", err)
					sess.CloseWithError(err)
					return
				}
			}
		}
	}()
}
