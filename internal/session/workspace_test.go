package session

import (
	"testing"
	"time"
)

func TestRepeatedRemovalDoesNotExtendReconnectDeadline(t *testing.T) {
	s := newTestSession("owner")
	s.AddWebSocket("conn", nil, false)
	s.RemoveWebSocket("conn")
	before := s.Info().ExpiresAt
	time.Sleep(time.Millisecond)
	s.RemoveWebSocket("conn")
	if !s.Info().ExpiresAt.Equal(before) {
		t.Fatal("duplicate removal reset grace period")
	}
}
