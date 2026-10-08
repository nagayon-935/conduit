package session

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/recording"
	"golang.org/x/crypto/ssh"
)

const (
	ToClientBufSize    = 256
	FromClientBufSize  = 64
	tokenPreviewLength = 8 // characters shown in Info() before "..."
)

type SessionState int

const (
	StateConnected SessionState = iota
	StateDisconnected
	StateTerminated
)

type SessionInfo struct {
	ID                 string    `json:"id"` // non-secret admin identifier (= LogID)
	Token              string    `json:"token"`
	Host               string    `json:"host"`
	Port               int       `json:"port"`
	User               string    `json:"user"`
	State              string    `json:"state"`
	CreatedAt          time.Time `json:"created_at"`
	ExpiresAt          time.Time `json:"expires_at"`
	WSCount            int       `json:"ws_count"`
	ViewerCount        int       `json:"viewer_count"`
	GracePeriodSeconds int64     `json:"grace_period_seconds"`
	EndReason          string    `json:"end_reason,omitempty"`
}

type Session struct {
	Token     string
	LogID     string
	OnClose   func(err error)
	Host      string
	Port      int
	User      string
	CreatedAt time.Time
	ExpiresAt time.Time

	SSHClient  *ssh.Client
	SSHSession *ssh.Session
	Stdin      io.WriteCloser
	Stdout     io.Reader

	ToClient   chan []byte
	FromClient chan []byte

	// Recorder captures terminal output as asciinema v2. May be nil when recording is disabled.
	Recorder *recording.Recorder

	EndReason string
	State     SessionState
	done      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc

	gracePeriod time.Duration

	lastActivity time.Time // last stdin forward to the SSH process; guarded by mu

	wsConns   map[string]*SafeConn
	wsNotify  map[string]chan struct{}
	wsRoles   map[string]bool   // connID -> readOnly
	wsShares  map[string]string // connID -> share capability
	pumpsOnce sync.Once

	mu sync.RWMutex
}

func NewSession(token, host string, port int, user string, client *ssh.Client, sshSess *ssh.Session, stdin io.WriteCloser, stdout io.Reader, gracePeriod time.Duration) *Session {
	now := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		Token:        token,
		Host:         host,
		Port:         port,
		User:         user,
		CreatedAt:    now,
		ExpiresAt:    now.Add(gracePeriod),
		SSHClient:    client,
		SSHSession:   sshSess,
		Stdin:        stdin,
		Stdout:       stdout,
		ToClient:     make(chan []byte, ToClientBufSize),
		FromClient:   make(chan []byte, FromClientBufSize),
		State:        StateDisconnected,
		done:         make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,
		gracePeriod:  gracePeriod,
		lastActivity: now,
		wsConns:      make(map[string]*SafeConn),
		wsNotify:     make(map[string]chan struct{}),
		wsRoles:      make(map[string]bool),
		wsShares:     make(map[string]string),
	}
}

func (s *Session) Close() {
	s.CloseWithError(nil)
}

func (s *Session) CloseWithError(err error) {
	reason := "SSH セッションが終了しました。"
	if err != nil {
		reason = "SSH 接続でエラーが発生したため終了しました。"
	}
	s.closeWithReason(err, reason)
}

func (s *Session) CloseWithReason(reason string) { s.closeWithReason(nil, reason) }

func (s *Session) closeWithReason(err error, reason string) {
	s.mu.Lock()
	if s.State == StateTerminated {
		s.mu.Unlock()
		return
	}
	s.State = StateTerminated
	s.EndReason = reason

	select {
	case <-s.done:
	default:
		close(s.done)
	}
	s.cancel()
	onClose := s.OnClose
	s.mu.Unlock()

	if s.SSHSession != nil {
		_ = s.SSHSession.Close()
	}
	if s.SSHClient != nil {
		_ = s.SSHClient.Close()
	}
	if s.Recorder != nil {
		if recErr := s.Recorder.Close(); recErr != nil {
			slog.Warn("session: close recorder failed", "error", recErr)
		}
	}
	if onClose != nil {
		onClose(err)
	}
}

func (s *Session) IsExpired() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.State == StateConnected {
		return false
	}
	return time.Now().After(s.ExpiresAt)
}

func (s *Session) Done() <-chan struct{} {
	return s.done
}

func (s *Session) Context() context.Context {
	return s.ctx
}

func (s *Session) StartOnce(fn func()) {
	s.pumpsOnce.Do(fn)
}

// TouchActivity records that stdin was forwarded to the SSH process.
func (s *Session) TouchActivity() {
	s.mu.Lock()
	s.lastActivity = time.Now()
	s.mu.Unlock()
}

// IdleDuration reports how long the session has gone without stdin input.
func (s *Session) IdleDuration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.lastActivity)
}

// AddWebSocket registers a WebSocket connection with the session.
// readOnly=true means the connection may only receive output (no stdin forwarding).
func (s *Session) AddWebSocket(connID string, ws *websocket.Conn, readOnly bool) <-chan struct{} {
	notify, err := s.attachWebSocket(connID, ws, readOnly, "")
	if err != nil {
		notify = make(chan struct{})
		close(notify)
	}
	return notify
}

func (s *Session) attachWebSocket(connID string, ws *websocket.Conn, readOnly bool, shareToken string) (chan struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.State == StateTerminated {
		return nil, fmt.Errorf("session: session is terminated")
	}
	if s.State != StateConnected && time.Now().After(s.ExpiresAt) {
		return nil, fmt.Errorf("session: session has expired")
	}
	if _, exists := s.wsConns[connID]; exists {
		return nil, fmt.Errorf("session: duplicate connection ID")
	}
	notify := make(chan struct{})
	s.wsConns[connID] = NewSafeConn(ws)
	s.wsNotify[connID] = notify
	s.wsRoles[connID] = readOnly
	s.wsShares[connID] = shareToken
	s.State = StateConnected
	s.ExpiresAt = time.Now().Add(s.gracePeriod)
	return notify, nil
}

// CloseShareConnections ends only viewers admitted by the revoked link.
func (s *Session) CloseShareConnections(shareToken string) {
	s.mu.RLock()
	conns := make(map[string]*SafeConn)
	for id, token := range s.wsShares {
		if token == shareToken {
			conns[id] = s.wsConns[id]
		}
	}
	s.mu.RUnlock()
	for id, ws := range conns {
		if ws.Conn != nil {
			_ = ws.WriteJSON(map[string]string{"type": "exit", "reason": "共有リンクが無効になりました。"})
			_ = ws.Close()
		}
		s.RemoveWebSocket(id)
	}
}

// IsReadOnly reports whether the connection identified by connID is read-only.
func (s *Session) IsReadOnly(connID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.wsRoles[connID]
}

func (s *Session) RemoveWebSocket(connID string) {
	s.mu.Lock()
	notify := s.wsNotify[connID]
	if notify == nil {
		s.mu.Unlock()
		return
	}
	delete(s.wsConns, connID)
	delete(s.wsNotify, connID)
	delete(s.wsRoles, connID)
	delete(s.wsShares, connID)
	if len(s.wsConns) == 0 && s.State != StateTerminated {
		s.State = StateDisconnected
		s.ExpiresAt = time.Now().Add(s.gracePeriod)
	}
	s.mu.Unlock()

	if notify != nil {
		close(notify)
	}
	slog.Debug("websocket removed from session", "connID", connID)
}

func (s *Session) BroadcastToWebSockets(msgType int, data []byte) {
	s.mu.RLock()
	type entry struct {
		id string
		ws *SafeConn
	}
	conns := make([]entry, 0, len(s.wsConns))
	for id, ws := range s.wsConns {
		conns = append(conns, entry{id, ws})
	}
	s.mu.RUnlock()

	for _, c := range conns {
		if err := c.ws.WriteMessage(msgType, data); err != nil {
			slog.Warn("BroadcastToWebSockets: write error, removing connection", "connID", c.id, "error", err)
			s.RemoveWebSocket(c.id)
		}
	}
}

// GetSafeConn returns the SafeConn wrapper for the given connection ID.
// Returns nil if not found.
func (s *Session) GetSafeConn(connID string) *SafeConn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.wsConns[connID]
}

func (s *Session) ActiveWSCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.wsConns)
}

func (s *Session) Info() SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stateStr := "connected"
	switch s.State {
	case StateDisconnected:
		stateStr = "disconnected"
	case StateTerminated:
		stateStr = "terminated"
	}

	tok := s.Token
	if len(tok) > tokenPreviewLength {
		tok = tok[:tokenPreviewLength] + "..."
	}

	viewers := 0
	for _, ro := range s.wsRoles {
		if ro {
			viewers++
		}
	}
	return SessionInfo{
		ID:                 s.LogID,
		Token:              tok,
		Host:               s.Host,
		Port:               s.Port,
		User:               s.User,
		State:              stateStr,
		CreatedAt:          s.CreatedAt,
		ExpiresAt:          s.ExpiresAt,
		WSCount:            len(s.wsConns),
		ViewerCount:        viewers,
		GracePeriodSeconds: int64(s.gracePeriod / time.Second),
		EndReason:          s.EndReason,
	}
}
