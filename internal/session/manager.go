package session

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/config"
)

// Manager orchestrates session lifecycle on top of a Store.
type Manager struct {
	store  *Store
	config *config.Config
}

// NewManager constructs a Manager backed by a fresh Store.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		store:  NewStore(),
		config: cfg,
	}
}

// Create registers sess in the store. The session must have a non-empty Token.
func (m *Manager) Create(sess *Session) error {
	if sess.Token == "" {
		return fmt.Errorf("session: token must not be empty")
	}
	m.store.Set(sess.Token, sess)
	slog.Info("session created", "token", sess.Token, "expires_at", sess.ExpiresAt)
	return nil
}

// Get retrieves a session by token. Returns an error if not found or already terminated.
func (m *Manager) Get(token string) (*Session, error) {
	sess, ok := m.store.Get(token)
	if !ok {
		return nil, fmt.Errorf("session: token not found")
	}
	sess.mu.RLock()
	state := sess.State
	sess.mu.RUnlock()
	if state == StateTerminated {
		return nil, fmt.Errorf("session: session is terminated")
	}
	return sess, nil
}

// Attach links ws to the session identified by token using the given connID.
// readOnly=true prevents the connection from sending stdin to the SSH process.
// It returns the session, a channel that is closed when this connection is removed,
// and an error if the session does not exist, is terminated, or has expired.
func (m *Manager) Attach(token, connID string, ws *websocket.Conn, readOnly bool) (*Session, <-chan struct{}, error) {
	sess, err := m.Get(token)
	if err != nil {
		return nil, nil, err
	}
	if sess.IsExpired() {
		_ = m.Terminate(token)
		return nil, nil, fmt.Errorf("session: session has expired")
	}
	removedCh := sess.AddWebSocket(connID, ws, readOnly)
	select {
	case <-removedCh:
		return nil, nil, fmt.Errorf("session: duplicate, terminated or expired connection")
	default:
	}
	if sess.GetSafeConn(connID) == nil {
		return nil, nil, fmt.Errorf("session: terminated or expired while attaching")
	}
	mode := "read-write"
	if readOnly {
		mode = "read-only"
	}
	slog.Info("websocket attached to session", "token", token, "connID", connID, "mode", mode)
	return sess, removedCh, nil
}

// Terminate closes the session and removes it from the store.
func (m *Manager) Terminate(token string) error {
	return m.TerminateWithReason(token, "owner_terminated")
}
func (m *Manager) TerminateWithReason(token, reason string) error {
	sess, ok := m.store.Get(token)
	if !ok {
		return fmt.Errorf("session: token not found for termination")
	}
	sess.CloseWithReason(reason)
	m.store.Delete(token)
	slog.Info("session terminated", "token", token)
	return nil
}

// TerminateByID force-closes the session whose non-secret ID (LogID) matches id.
// Used by the admin API, which only sees truncated capability tokens via List().
func (m *Manager) TerminateByID(id string) error {
	var token string
	m.store.Range(func(t string, sess *Session) bool {
		if sess.LogID == id {
			token = t
			return false
		}
		return true
	})
	if token == "" {
		return fmt.Errorf("session: id not found for termination")
	}
	return m.Terminate(token)
}

// StartGC launches a background goroutine that periodically reaps expired sessions
// and stale SSH transports.
func (m *Manager) StartGC(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(m.config.SessionGCInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("session GC stopped")
				return
			case <-ticker.C:
				m.gc()
			}
		}
	}()
}

// gc iterates the store and terminates any sessions that have expired
// (disconnected past the grace period) or exceeded the idle timeout
// (no stdin activity, even while still Connected).
func (m *Manager) gc() {
	idleTimeout := m.config.IdleTimeout

	var expiredSessions, idleSessions []string
	m.store.Range(func(token string, sess *Session) bool {
		if sess.IsExpired() {
			expiredSessions = append(expiredSessions, token)
			return true
		}
		limit := sess.IdleLimit(idleTimeout)
		if limit > 0 && sess.IdleDuration() >= limit {
			idleSessions = append(idleSessions, token)
		}
		return true
	})
	for _, token := range expiredSessions {
		slog.Info("GC: reaping expired session", "token", token)
		_ = m.TerminateWithReason(token, "reconnect_expired")
	}
	for _, token := range idleSessions {
		slog.Info("GC: closing idle session", "token", token, "idle_timeout", idleTimeout)
		_ = m.TerminateWithReason(token, "idle_timeout")
	}

}

// List returns a snapshot of info for all sessions currently in the store.
func (m *Manager) List() []SessionInfo {
	var infos []SessionInfo
	m.store.Range(func(_ string, sess *Session) bool {
		infos = append(infos, sess.Info())
		return true
	})
	if infos == nil {
		infos = []SessionInfo{}
	}
	return infos
}

// ForgetTerminated removes a finished transport without calling Close recursively.
func (m *Manager) ForgetTerminated(id string) {
	s, ok := m.store.Get(id)
	if !ok {
		return
	}
	s.mu.RLock()
	closed := s.State == StateTerminated
	s.mu.RUnlock()
	if closed {
		m.store.Delete(id)
	}
}
