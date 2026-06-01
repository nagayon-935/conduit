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
