package session

import (
	"testing"
	"time"
)

// TestSession_IdleDuration_FreshSessionIsNearZero verifies a newly created
// session reports a near-zero idle duration (lastActivity initialized to now).
func TestSession_IdleDuration_FreshSessionIsNearZero(t *testing.T) {
	t.Parallel()
	s := newTestSession("idle-fresh")
	if got := s.IdleDuration(); got > 100*time.Millisecond {
		t.Errorf("IdleDuration() = %v, want near 0 for a freshly created session", got)
	}
}

// TestSession_IdleDuration_GrowsOverTime verifies IdleDuration increases while
// no activity is recorded.
func TestSession_IdleDuration_GrowsOverTime(t *testing.T) {
	t.Parallel()
	s := newTestSession("idle-grows")
	time.Sleep(30 * time.Millisecond)
	if got := s.IdleDuration(); got < 30*time.Millisecond {
		t.Errorf("IdleDuration() = %v, want >= 30ms", got)
	}
}

// TestSession_TouchActivity_ResetsIdleDuration verifies TouchActivity resets
// the idle clock back to near zero.
func TestSession_TouchActivity_ResetsIdleDuration(t *testing.T) {
	t.Parallel()
	s := newTestSession("idle-touch")
	time.Sleep(30 * time.Millisecond)
	s.TouchActivity()
	if got := s.IdleDuration(); got > 15*time.Millisecond {
		t.Errorf("IdleDuration() after TouchActivity = %v, want near 0", got)
	}
}
