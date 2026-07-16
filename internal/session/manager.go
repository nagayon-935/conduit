package session

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/config"
	pkgtoken "github.com/nagayon-935/conduit/pkg/token"
)

const defaultShareTTL = 4 * time.Hour

type shareEntry struct {
	sessionToken string
	expiresAt    time.Time
}

// Manager orchestrates session lifecycle on top of a Store.
type Manager struct {
	store    *Store
	config   *config.Config
	shares   map[string]*shareEntry // shareToken -> entry
	sharesMu sync.RWMutex
}

// NewManager constructs a Manager backed by a fresh Store.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		store:  NewStore(),
		config: cfg,
		shares: make(map[string]*shareEntry),
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
	mode := "read-write"
	if readOnly {
		mode = "read-only"
	}
	slog.Info("websocket attached to session", "token", token, "connID", connID, "mode", mode)
	return sess, removedCh, nil
}

// Terminate closes the session and removes it from the store.
func (m *Manager) Terminate(token string) error {
	sess, ok := m.store.Get(token)
	if !ok {
		return fmt.Errorf("session: token not found for termination")
	}
	sess.Close()
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

// Share creates a read-only share token for the session identified by sessionToken.
// The token expires after defaultShareTTL.
func (m *Manager) Share(sessionToken string) (shareToken string, expiresAt time.Time, err error) {
	if _, err = m.Get(sessionToken); err != nil {
		return "", time.Time{}, fmt.Errorf("session: cannot share: %w", err)
	}
	tok, err := pkgtoken.Generate()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("session: generate share token: %w", err)
	}
	exp := time.Now().Add(defaultShareTTL)

	m.sharesMu.Lock()
	m.shares[tok] = &shareEntry{sessionToken: sessionToken, expiresAt: exp}
	m.sharesMu.Unlock()

	slog.Info("share token created", "session", sessionToken, "share_token", tok[:8]+"...", "expires_at", exp)
	return tok, exp, nil
}

// ResolveShare resolves a share token to the underlying session token.
// Returns the session token and ok=true when the token is valid and unexpired.
func (m *Manager) ResolveShare(shareToken string) (sessionToken string, ok bool) {
	m.sharesMu.RLock()
	e, found := m.shares[shareToken]
	m.sharesMu.RUnlock()
	if !found || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.sessionToken, true
}

// RevokeShare invalidates a share token.
func (m *Manager) RevokeShare(shareToken string) {
	m.sharesMu.Lock()
	delete(m.shares, shareToken)
	m.sharesMu.Unlock()
	slog.Info("share token revoked", "share_token", shareToken[:min(8, len(shareToken))]+"...")
}

// StartGC launches a background goroutine that periodically reaps expired sessions
// and expired share tokens.
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
// (no stdin activity, even while still Connected), and purges expired
// share tokens.
func (m *Manager) gc() {
	idleTimeout := m.config.IdleTimeout

	var expiredSessions, idleSessions []string
	m.store.Range(func(token string, sess *Session) bool {
		if sess.IsExpired() {
			expiredSessions = append(expiredSessions, token)
			return true
		}
		if idleTimeout > 0 && sess.IdleDuration() >= idleTimeout {
			idleSessions = append(idleSessions, token)
		}
		return true
	})
	for _, token := range expiredSessions {
		slog.Info("GC: reaping expired session", "token", token)
		_ = m.Terminate(token)
	}
	for _, token := range idleSessions {
		slog.Info("GC: closing idle session", "token", token, "idle_timeout", idleTimeout)
		_ = m.Terminate(token)
	}

	now := time.Now()
	var expiredShares []string
	m.sharesMu.RLock()
	for tok, e := range m.shares {
		if now.After(e.expiresAt) {
			expiredShares = append(expiredShares, tok)
		}
	}
	m.sharesMu.RUnlock()

	if len(expiredShares) > 0 {
		m.sharesMu.Lock()
		for _, tok := range expiredShares {
			delete(m.shares, tok)
		}
		m.sharesMu.Unlock()
		slog.Info("GC: purged expired share tokens", "count", len(expiredShares))
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
