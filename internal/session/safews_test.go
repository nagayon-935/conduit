package session

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The pipe has no reader, so writes block regardless of TCP buffer capacity.
type stalledWriteConn struct {
	net.Conn
	pipe    net.Conn
	stalled bool
}

func (c *stalledWriteConn) Write(p []byte) (int, error) {
	if c.stalled {
		return c.pipe.Write(p)
	}
	return c.Conn.Write(p)
}

func (c *stalledWriteConn) SetWriteDeadline(deadline time.Time) error {
	return errors.Join(c.pipe.SetWriteDeadline(deadline), c.Conn.SetWriteDeadline(deadline))
}

func (c *stalledWriteConn) Close() error {
	return errors.Join(c.pipe.Close(), c.Conn.Close())
}

type stalledResponseWriter struct {
	http.ResponseWriter
	pipe net.Conn
	conn *stalledWriteConn
}

func (w *stalledResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := w.ResponseWriter.(http.Hijacker).Hijack()
	if err != nil {
		return nil, nil, err
	}
	w.conn = &stalledWriteConn{Conn: conn, pipe: w.pipe}
	return w.conn, rw, nil
}

func newStalledSafeConn(t *testing.T) *SafeConn {
	t.Helper()
	pipe, peer := net.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	ready := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := &stalledResponseWriter{ResponseWriter: w, pipe: pipe}
		ws, err := (&websocket.Upgrader{}).Upgrade(writer, r, nil)
		if err != nil {
			ready <- nil
			return
		}
		writer.conn.stalled = true
		ready <- ws
	}))
	t.Cleanup(server.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ws := <-ready
	if ws == nil {
		t.Fatal("websocket upgrade failed")
	}
	t.Cleanup(func() { _ = ws.Close() })
	return NewSafeConn(ws)
}

func TestSafeConn_StalledWritesHaveDeadlines(t *testing.T) {
	for _, method := range []string{"message", "json", "explicit-deadline"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			ws := newStalledSafeConn(t)
			result := make(chan error, 1)
			go func() {
				switch method {
				case "message":
					result <- ws.WriteMessage(websocket.TextMessage, []byte("exit"))
				case "json":
					result <- ws.WriteJSON(map[string]string{"type": "exit"})
				default:
					result <- ws.WriteWithDeadline(time.Now().Add(25*time.Millisecond), websocket.TextMessage, []byte("pong"))
				}
			}()
			select {
			case err := <-result:
				var timeout net.Error
				if !errors.As(err, &timeout) || !timeout.Timeout() {
					t.Errorf("write error = %v, want a timeout", err)
				}
			case <-time.After(time.Second):
				t.Fatal("WebSocket write blocked without a deadline")
			}
		})
	}
}
