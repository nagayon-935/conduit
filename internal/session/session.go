package session

import (
	"context"
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
	GracePeriodSeconds int64     `json:"grace_period_seconds"`
	EndReason          string    `json:"end_reason,omitempty"`
	OwnerUserID        string    `json:"owner_user_id"`
	TargetAccountID    string    `json:"target_account_id"`
	Recording          bool      `json:"recording_enabled"`
	ID                 string    `json:"id"` // non-secret admin identifier (= LogID)
	Token              string    `json:"token,omitempty"`
	Host               string    `json:"host"`
	Port               int       `json:"port"`
	User               string    `json:"user"`
	State              string    `json:"state"`
	CreatedAt          time.Time `json:"created_at"`
	ExpiresAt          time.Time `json:"expires_at"`
	WSCount            int       `json:"ws_count"`
	ViewerCount        int       `json:"viewer_count"`
}

type Session struct {
	EndReason       string
	Token           string
	OwnerUserID     string
	TargetAccountID string
	LogID           string
	OnClose         func(err error)
	Host            string
	Port            int
	User            string
	CreatedAt       time.Time
	ExpiresAt       time.Time

	SSHClient  *ssh.Client
	SSHSession *ssh.Session
	Stdin      io.WriteCloser
	Stdout     io.Reader

	ToClient   chan []byte
	FromClient chan []byte

	// Recorder captures terminal output as asciinema v2. May be nil when recording is disabled.
	Recorder *recording.Recorder

	State  SessionState
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc

	gracePeriod      time.Duration
	idleLimit        time.Duration
	configuredLimits bool

	lastActivity time.Time // last stdin forward to the SSH process; guarded by mu

	wsConns   map[string]*SafeConn
	wsNotify  map[string]chan struct{}
	wsRoles   map[string]bool // connID -> readOnly
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
	}
}

func (s *Session) Close() {
	s.CloseWithError(nil)
}

func (s *Session) CloseWithError(err error) {
	reason := "ssh_exit"
	if err != nil {
		reason = "ssh_error"
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
	s.mu.Lock()
	defer s.mu.Unlock()

	notify := make(chan struct{})
	if s.State == StateTerminated || (s.State != StateConnected && time.Now().After(s.ExpiresAt)) || s.wsConns[connID] != nil {
		close(notify)
		return notify
	}
	s.wsConns[connID] = NewSafeConn(ws)
	s.wsNotify[connID] = notify
	s.wsRoles[connID] = readOnly
	if !readOnly {
		s.State = StateConnected
		s.ExpiresAt = time.Now().Add(s.gracePeriod)
	}
	return notify
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
	wasWriter := notify != nil && !s.wsRoles[connID]
	delete(s.wsConns, connID)
	delete(s.wsNotify, connID)
	delete(s.wsRoles, connID)
	writers := 0
	for _, ro := range s.wsRoles {
		if !ro {
			writers++
		}
	}
	if wasWriter && writers == 0 && s.State != StateTerminated {
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

	if s.OwnerUserID != "" {
		tok = ""
	}
	viewers := 0
	for _, ro := range s.wsRoles {
		if ro {
			viewers++
		}
	}
	return SessionInfo{
		GracePeriodSeconds: int64(s.gracePeriod / time.Second), EndReason: s.EndReason,
		ID:          s.LogID,
		OwnerUserID: s.OwnerUserID, TargetAccountID: s.TargetAccountID, Recording: s.Recorder != nil,
		Token:       tok,
		Host:        s.Host,
		Port:        s.Port,
		User:        s.User,
		State:       stateStr,
		CreatedAt:   s.CreatedAt,
		ExpiresAt:   s.ExpiresAt,
		WSCount:     len(s.wsConns),
		ViewerCount: viewers,
	}
}

func (s *Session) ConfigureLimits(grace, idle time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.State == StateDisconnected {
		s.ExpiresAt = s.ExpiresAt.Add(grace - s.gracePeriod)
	}
	s.gracePeriod = grace
	s.idleLimit = idle
	s.configuredLimits = true
}
func (s *Session) IdleLimit(fallback time.Duration) time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.configuredLimits {
		return s.idleLimit
	}
	return fallback
}
