package session

import (
	"testing"
	"time"

	"github.com/nagayon-935/conduit/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		GracePeriod:       15 * time.Minute,
		SessionGCInterval: 1 * time.Minute,
	}
}

// newTestSession builds a minimal Session with the given token (no real SSH client/session).
func newTestSession(token string) *Session {
	return NewSession(token, "", 0, "", nil, nil, nil, nil, 15*time.Minute)
}

func TestSessionManager_CreateAndGet(t *testing.T) {
	t.Parallel()

	m := NewManager(testConfig())
	sess := newTestSession("tok-1")
	if err := m.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := m.Get("tok-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Token != "tok-1" {
		t.Errorf("token mismatch: got %q, want %q", got.Token, "tok-1")
	}
}

func TestSessionManager_GetNonExistent(t *testing.T) {
	t.Parallel()

	m := NewManager(testConfig())
	_, err := m.Get("does-not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent token, got nil")
	}
}

func TestSessionManager_Attach(t *testing.T) {
	t.Parallel()

	m := NewManager(testConfig())
	sess := newTestSession("tok-attach")
	if err := m.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Attach nil WebSocket – AddWebSocket(nil) is valid and simply records nil.
	got, _, err := m.Attach("tok-attach", "conn1", nil, false)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if got.State != StateConnected {
		t.Errorf("state: got %v, want StateConnected", got.State)
	}
}

func TestSessionManager_AttachExpiredSession(t *testing.T) {
	t.Parallel()

	m := NewManager(testConfig())
	sess := newTestSession("tok-expired")
	// Simulate a disconnected session whose grace period has elapsed.
	sess.State = StateDisconnected
	sess.ExpiresAt = time.Now().Add(-1 * time.Second)
	if err := m.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, _, err := m.Attach("tok-expired", "conn1", nil, false)
	if err == nil {
		t.Fatal("expected error for expired session, got nil")
	}
}

func TestSessionManager_Terminate(t *testing.T) {
	t.Parallel()

	m := NewManager(testConfig())
	sess := newTestSession("tok-term")
	if err := m.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := m.Terminate("tok-term"); err != nil {
		t.Fatalf("Terminate: %v", err)
	}

	// After termination, Get should fail because the session is removed from the store.
	_, err := m.Get("tok-term")
	if err == nil {
		t.Fatal("expected error after termination, got nil")
	}
}

func TestSessionManager_GC(t *testing.T) {
	t.Parallel()

	m := NewManager(testConfig())

	// Create 3 sessions.
	s1 := newTestSession("gc-1")
	s2 := newTestSession("gc-2")
	s3 := newTestSession("gc-3")

	// Simulate disconnected sessions whose grace period has elapsed.
	s1.State = StateDisconnected
	s1.ExpiresAt = time.Now().Add(-1 * time.Second)
	s2.State = StateDisconnected
	s2.ExpiresAt = time.Now().Add(-1 * time.Second)

	for _, s := range []*Session{s1, s2, s3} {
		if err := m.Create(s); err != nil {
			t.Fatalf("Create %s: %v", s.Token, err)
		}
	}

	// Run GC directly.
	m.gc()

	// gc-1 and gc-2 should be gone.
	if _, err := m.Get("gc-1"); err == nil {
		t.Error("expected gc-1 to be reaped, but Get succeeded")
	}
	if _, err := m.Get("gc-2"); err == nil {
		t.Error("expected gc-2 to be reaped, but Get succeeded")
	}

	// gc-3 should still exist.
	if _, err := m.Get("gc-3"); err != nil {
		t.Errorf("expected gc-3 to survive GC, got: %v", err)
	}
}

func TestSessionManager_GracePeriodReconnect(t *testing.T) {
	t.Parallel()

	m := NewManager(testConfig())
	sess := newTestSession("tok-grace")
	if err := m.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Attach then detach (simulate disconnect).
	if _, _, err := m.Attach("tok-grace", "conn1", nil, false); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	sess.RemoveWebSocket("conn1")

	// Verify the session is still in the store (grace period hasn't elapsed).
	if _, err := m.Get("tok-grace"); err != nil {
		t.Fatalf("session should still exist within grace period: %v", err)
	}

	// Reconnect before grace period expires.
	got, _, err := m.Attach("tok-grace", "conn2", nil, false)
	if err != nil {
		t.Fatalf("second Attach (reconnect): %v", err)
	}
	if got.State != StateConnected {
		t.Errorf("state after reconnect: got %v, want StateConnected", got.State)
	}
}

// --- Session method tests ---

// TestSession_Done_NotNil checks that Done() returns a non-nil channel.
func TestSession_Done_NotNil(t *testing.T) {
	t.Parallel()
	s := newTestSession("done-test")
	if s.Done() == nil {
		t.Fatal("Done() returned nil channel")
	}
}

// TestSession_Done_ClosedAfterClose checks that Done() is closed after Close().
func TestSession_Done_ClosedAfterClose(t *testing.T) {
	t.Parallel()
	s := newTestSession("done-close-test")
	s.Close()
	select {
	case <-s.Done():
		// expected: channel closed
	default:
		t.Fatal("Done() channel not closed after Close()")
	}
}

// TestSession_ActiveWSCount_InitiallyZero checks that a new session has no WebSocket connections.
func TestSession_ActiveWSCount_InitiallyZero(t *testing.T) {
	t.Parallel()
	s := newTestSession("ws-count-test")
	if count := s.ActiveWSCount(); count != 0 {
		t.Errorf("expected 0 WebSocket connections on new session, got %d", count)
	}
}

// TestSession_Close_Idempotent verifies that calling Close() twice does not panic.
func TestSession_Close_Idempotent(t *testing.T) {
	t.Parallel()
	s := newTestSession("close-idempotent-test")
	s.Close()
	// Second call must not panic (double-close of done channel would panic without guard).
	s.Close()
	if s.State != StateTerminated {
		t.Errorf("state = %v, want StateTerminated", s.State)
	}
}

// TestSession_IsExpired_False confirms a freshly created session is not expired.
func TestSession_IsExpired_False(t *testing.T) {
	t.Parallel()
	s := newTestSession("not-expired")
	if s.IsExpired() {
		t.Error("newly created session should not be expired")
	}
}

// TestSession_IsExpired_True confirms a disconnected session with a past ExpiresAt is expired.
func TestSession_IsExpired_True(t *testing.T) {
	t.Parallel()
	s := newTestSession("expired")
	// Must be StateDisconnected; connected sessions are never expired.
	s.State = StateDisconnected
	s.ExpiresAt = time.Now().Add(-1 * time.Second)
	if !s.IsExpired() {
		t.Error("backdated disconnected session should be expired")
	}
}

// TestSession_RemoveWebSocket_SetsStateDisconnected verifies RemoveWebSocket transitions the state.
func TestSession_RemoveWebSocket_SetsStateDisconnected(t *testing.T) {
	t.Parallel()
	s := newTestSession("detach-test")
	s.AddWebSocket("conn1", nil, false)
	s.RemoveWebSocket("conn1")
	if s.State != StateDisconnected {
		t.Errorf("state = %v, want StateDisconnected", s.State)
	}
}

// TestSessionManager_CreateEmptyToken verifies that Create rejects a session with no token.
func TestSessionManager_CreateEmptyToken(t *testing.T) {
	t.Parallel()
	m := NewManager(testConfig())
	s := newTestSession("") // empty token
	if err := m.Create(s); err == nil {
		t.Fatal("expected error for empty-token Create, got nil")
	}
}

// TestSessionManager_TerminateNonExistent verifies that Terminate on a missing token returns an error.
func TestSessionManager_TerminateNonExistent(t *testing.T) {
	t.Parallel()
	m := NewManager(testConfig())
	if err := m.Terminate("no-such-token"); err == nil {
		t.Fatal("expected error for Terminate of non-existent session, got nil")
	}
}

// ── Share token tests ────────────────────────────────────────────────────────

func TestShare_CreateAndResolve(t *testing.T) {
	t.Parallel()
	m := NewManager(testConfig())
	sess := newTestSession("tok-share")
	_ = m.Create(sess)

	shareToken, expiresAt, err := m.Share("tok-share")
	if err != nil {
		t.Fatalf("Share: %v", err)
	}
	if shareToken == "" {
		t.Fatal("expected non-empty share token")
	}
	if expiresAt.Before(time.Now()) {
		t.Error("expiresAt should be in the future")
	}

	sessionToken, ok := m.ResolveShare(shareToken)
	if !ok {
		t.Fatal("ResolveShare: expected ok=true")
	}
	if sessionToken != "tok-share" {
		t.Errorf("sessionToken = %q, want %q", sessionToken, "tok-share")
	}
}

func TestShare_Revoke(t *testing.T) {
	t.Parallel()
	m := NewManager(testConfig())
	sess := newTestSession("tok-revoke")
	_ = m.Create(sess)

	shareToken, _, _ := m.Share("tok-revoke")
	m.RevokeShare(shareToken)

	_, ok := m.ResolveShare(shareToken)
	if ok {
		t.Fatal("expected ResolveShare to fail after revocation")
	}
}

func TestShare_NonExistentSession(t *testing.T) {
	t.Parallel()
	m := NewManager(testConfig())
	_, _, err := m.Share("no-such-session")
	if err == nil {
		t.Fatal("expected error when sharing non-existent session")
	}
}

func TestShare_InvalidToken(t *testing.T) {
	t.Parallel()
	m := NewManager(testConfig())
	_, ok := m.ResolveShare("totally-fake-token")
	if ok {
		t.Fatal("expected ResolveShare to return ok=false for unknown token")
	}
}

// ── TerminateByID ────────────────────────────────────────────────────────────

// TestSessionManager_TerminateByID_Success verifies a session can be killed
// using its non-secret LogID (used by the admin API, which never sees the
// full capability token from GET /api/sessions).
func TestSessionManager_TerminateByID_Success(t *testing.T) {
	t.Parallel()
	m := NewManager(testConfig())
	sess := newTestSession("tok-by-id")
	sess.LogID = "log-abc"
	if err := m.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := m.TerminateByID("log-abc"); err != nil {
		t.Fatalf("TerminateByID: %v", err)
	}
	if _, err := m.Get("tok-by-id"); err == nil {
		t.Fatal("expected session removed after TerminateByID")
	}
}

// TestSessionManager_TerminateByID_NotFound verifies an unknown ID returns an error.
func TestSessionManager_TerminateByID_NotFound(t *testing.T) {
	t.Parallel()
	m := NewManager(testConfig())
	if err := m.TerminateByID("no-such-id"); err == nil {
		t.Fatal("expected error for TerminateByID with unknown id")
	}
}

// ── Idle timeout GC ──────────────────────────────────────────────────────────

func idleTestConfig(idleTimeout time.Duration) *config.Config {
	return &config.Config{
		GracePeriod:       15 * time.Minute,
		SessionGCInterval: time.Minute,
		IdleTimeout:       idleTimeout,
	}
}

// TestSessionManager_GC_IdleTimeout_TerminatesConnectedIdleSession verifies
// that gc() reaps a Connected session once it exceeds IdleTimeout, independent
// of the grace-period (disconnect) expiry logic.
func TestSessionManager_GC_IdleTimeout_TerminatesConnectedIdleSession(t *testing.T) {
	t.Parallel()
	m := NewManager(idleTestConfig(10 * time.Millisecond))
	sess := newTestSession("idle-conn-1")
	if err := m.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, _, err := m.Attach("idle-conn-1", "conn1", nil, false); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	time.Sleep(20 * time.Millisecond) // exceed IdleTimeout; no TouchActivity called

	m.gc()

	if _, err := m.Get("idle-conn-1"); err == nil {
		t.Error("expected idle Connected session to be reaped by GC, but Get succeeded")
	}
}

// TestSessionManager_GC_IdleTimeout_SurvivesActiveSession verifies a session
// that recently touched activity survives GC even though it is well within
// the grace period.
func TestSessionManager_GC_IdleTimeout_SurvivesActiveSession(t *testing.T) {
	t.Parallel()
	m := NewManager(idleTestConfig(50 * time.Millisecond))
	sess := newTestSession("idle-active-1")
	if err := m.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, _, err := m.Attach("idle-active-1", "conn1", nil, false); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	sess.TouchActivity()

	m.gc()

	if _, err := m.Get("idle-active-1"); err != nil {
		t.Errorf("expected active session to survive GC, got: %v", err)
	}
}

// TestSessionManager_GC_IdleTimeoutDisabled verifies IdleTimeout=0 disables
// idle reaping entirely.
func TestSessionManager_GC_IdleTimeoutDisabled(t *testing.T) {
	t.Parallel()
	m := NewManager(idleTestConfig(0))
	sess := newTestSession("idle-disabled-1")
	if err := m.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, _, err := m.Attach("idle-disabled-1", "conn1", nil, false); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	m.gc()

	if _, err := m.Get("idle-disabled-1"); err != nil {
		t.Errorf("expected session to survive GC when IdleTimeout=0, got: %v", err)
	}
}

func TestShare_ReadOnlyAttach(t *testing.T) {
	t.Parallel()
	m := NewManager(testConfig())
	sess := newTestSession("tok-ro")
	_ = m.Create(sess)

	shareToken, _, _ := m.Share("tok-ro")
	sessionToken, ok := m.ResolveShare(shareToken)
	if !ok {
		t.Fatal("ResolveShare failed")
	}

	// Attach as read-only (nil ws is acceptable in unit tests).
	attachedSess, _, err := m.Attach(sessionToken, "viewer-conn", nil, true)
	if err != nil {
		t.Fatalf("Attach (read-only): %v", err)
	}
	if !attachedSess.IsReadOnly("viewer-conn") {
		t.Error("expected viewer connection to be read-only")
	}
}
