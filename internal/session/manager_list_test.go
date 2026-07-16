package session

import (
	"sort"
	"testing"
)

func TestManager_List_Empty(t *testing.T) {
	t.Parallel()

	m := NewManager(testConfig())
	got := m.List()
	if got == nil {
		t.Fatal("List() returned nil; want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("List() len = %d, want 0", len(got))
	}
}

func TestManager_List_ReturnsAllSessions(t *testing.T) {
	t.Parallel()

	m := NewManager(testConfig())
	for _, tok := range []string{"tok-a", "tok-b", "tok-c"} {
		if err := m.Create(newTestSession(tok)); err != nil {
			t.Fatalf("Create(%q): %v", tok, err)
		}
	}

	infos := m.List()
	if len(infos) != 3 {
		t.Fatalf("List() len = %d, want 3", len(infos))
	}

	tokens := make([]string, len(infos))
	for i, info := range infos {
		tokens[i] = info.Token
		// A freshly created session has no WebSocket attached → disconnected.
		if info.State != "disconnected" {
			t.Errorf("session %q state = %q, want disconnected", info.Token, info.State)
		}
	}
	sort.Strings(tokens)
	want := []string{"tok-a", "tok-b", "tok-c"}
	for i := range want {
		if tokens[i] != want[i] {
			t.Errorf("tokens[%d] = %q, want %q", i, tokens[i], want[i])
		}
	}
}

// TestManager_List_IncludesID verifies SessionInfo exposes the non-secret
// LogID as ID, so the admin UI can target a session without holding its full
// capability token.
func TestManager_List_IncludesID(t *testing.T) {
	t.Parallel()

	m := NewManager(testConfig())
	sess := newTestSession("tok-id")
	sess.LogID = "log-xyz"
	if err := m.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	infos := m.List()
	if len(infos) != 1 {
		t.Fatalf("List() len = %d, want 1", len(infos))
	}
	if infos[0].ID != "log-xyz" {
		t.Errorf("ID = %q, want %q", infos[0].ID, "log-xyz")
	}
}
