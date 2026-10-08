package session

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// SafeConn wraps a *websocket.Conn with a mutex to serialize writes.
// gorilla/websocket supports one concurrent reader and one concurrent writer,
// but NOT multiple concurrent writers. SafeConn enforces this constraint.
type SafeConn struct {
	*websocket.Conn
	mu sync.Mutex
}

// NewSafeConn wraps a raw *websocket.Conn.
func NewSafeConn(ws *websocket.Conn) *SafeConn {
	return &SafeConn{Conn: ws}
}

// wsWriteTimeout is the maximum time allowed for a single WebSocket write.
// A blocked write stalls the broadcast loop and eventually causes SSH output
// to be dropped, which manifests as a frozen terminal display.
const wsWriteTimeout = 250 * time.Millisecond

// WriteMessage acquires the write lock before writing.
// A write deadline is always set so that a slow or stuck client cannot block
// the broadcast loop indefinitely.
func (c *SafeConn) WriteMessage(messageType int, data []byte) error {
	return c.WriteWithDeadline(time.Now().Add(wsWriteTimeout), messageType, data)
}

// WriteJSON uses the same bounded write as terminal output, including exit frames.
func (c *SafeConn) WriteJSON(v any) error {
	return c.withWriteDeadline(time.Now().Add(wsWriteTimeout), func() error {
		return c.Conn.WriteJSON(v)
	})
}

// WriteWithDeadline sets a write deadline and writes a message under one lock.
func (c *SafeConn) WriteWithDeadline(deadline time.Time, messageType int, data []byte) error {
	return c.withWriteDeadline(deadline, func() error {
		return c.Conn.WriteMessage(messageType, data)
	})
}

func (c *SafeConn) withWriteDeadline(deadline time.Time, write func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.Conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	defer c.Conn.SetWriteDeadline(time.Time{})
	return write()
}
