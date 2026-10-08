package session

import (
	"testing"
	"time"
)

func TestRevocationOnlyRemovesViewersForThatLink(t *testing.T) {
	m := NewManager(testConfig())
	s := newTestSession("owner")
	_ = m.Create(s)
	_, ownerGone, err := m.Attach(s.Token, "owner-conn", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	a, _, _ := m.Share(s.Token)
	b, _, _ := m.Share(s.Token)
	_, aGone, err := m.AttachShared(a, "a-viewer", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, bGone, err := m.AttachShared(b, "b-viewer", nil)
	if err != nil {
		t.Fatal(err)
	}
	m.RevokeShare(a)
	select {
	case <-aGone:
	default:
		t.Fatal("revoked viewer not removed")
	}
	for _, ch := range []<-chan struct{}{ownerGone, bGone} {
		select {
		case <-ch:
			t.Fatal("unrelated connection removed")
		default:
		}
	}
	if s.Info().ViewerCount != 1 || s.ActiveWSCount() != 2 {
		t.Fatalf("counts: %+v", s.Info())
	}
	if _, _, err := m.AttachShared(a, "new-viewer", nil); err == nil {
		t.Fatal("revoked share attached")
	}
	s.Close()
	if _, _, err := m.AttachShared(b, "late-viewer", nil); err == nil {
		t.Fatal("terminated session attached")
	}
}

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
