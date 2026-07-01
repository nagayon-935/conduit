package connlog

import (
	"sync"
	"testing"
	"time"
)

func newTestSQLiteStore(t *testing.T, maxRows int) *SQLiteStore {
	t.Helper()
	path := t.TempDir() + "/test.db"
	s, err := NewSQLiteStore(path, maxRows)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	s.syncTrim = true
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSQLiteStore_AddAndList(t *testing.T) {
	t.Parallel()
	s := newTestSQLiteStore(t, 3)

	base := time.Now().Truncate(time.Millisecond)
	entries := []*Entry{
		{ID: "1", Host: "h1", Port: 22, User: "u1", ConnectedAt: base},
		{ID: "2", Host: "h2", Port: 22, User: "u2", ConnectedAt: base.Add(time.Millisecond)},
		{ID: "3", Host: "h3", Port: 22, User: "u3", ConnectedAt: base.Add(2 * time.Millisecond)},
		{ID: "4", Host: "h4", Port: 22, User: "u4", ConnectedAt: base.Add(3 * time.Millisecond)},
	}
	for _, e := range entries {
		s.Add(e)
	}

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, want 3 (capped at maxRows)", len(list))
	}
	if list[0].ID != "4" {
		t.Errorf("list[0].ID = %q, want %q (newest first)", list[0].ID, "4")
	}
	if list[2].ID != "2" {
		t.Errorf("list[2].ID = %q, want %q", list[2].ID, "2")
	}
}

func TestSQLiteStore_Empty(t *testing.T) {
	t.Parallel()
	s := newTestSQLiteStore(t, 10)
	list := s.List()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d entries", len(list))
	}
}

func TestSQLiteStore_UpdateError(t *testing.T) {
	t.Parallel()
	s := newTestSQLiteStore(t, 10)

	now := time.Now().Truncate(time.Millisecond)
	s.Add(&Entry{ID: "x", Host: "h", Port: 22, User: "u", ConnectedAt: now})
	s.UpdateError("x", "connection refused", now.Add(time.Second))

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}
	e := list[0]
	if e.Error != "connection refused" {
		t.Errorf("Error = %q, want %q", e.Error, "connection refused")
	}
	if e.DisconnectedAt == nil {
		t.Fatal("DisconnectedAt should not be nil after UpdateError")
	}
}

func TestSQLiteStore_UpdateDisconnected(t *testing.T) {
	t.Parallel()
	s := newTestSQLiteStore(t, 10)

	now := time.Now().Truncate(time.Millisecond)
	s.Add(&Entry{ID: "y", Host: "h", Port: 22, User: "u", ConnectedAt: now})
	s.UpdateDisconnected("y", now.Add(2*time.Second))

	list := s.List()
	if list[0].DisconnectedAt == nil {
		t.Fatal("DisconnectedAt should be set after UpdateDisconnected")
	}
	if list[0].Error != "" {
		t.Errorf("Error should be empty, got %q", list[0].Error)
	}
}

func TestSQLiteStore_RecordingPath(t *testing.T) {
	t.Parallel()
	s := newTestSQLiteStore(t, 10)

	s.Add(&Entry{
		ID: "r1", Host: "h", Port: 22, User: "u",
		ConnectedAt:   time.Now(),
		RecordingPath: "/recordings/r1.cast",
	})

	list := s.List()
	if list[0].RecordingPath != "/recordings/r1.cast" {
		t.Errorf("RecordingPath = %q, want %q", list[0].RecordingPath, "/recordings/r1.cast")
	}
}

func TestSQLiteStore_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	s := newTestSQLiteStore(t, 200)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := string(rune('a'+n%26)) + time.Now().Format("150405.000000000")
			s.Add(&Entry{ID: id, Host: "h", Port: 22, User: "u", ConnectedAt: time.Now()})
		}(i)
	}
	wg.Wait()

	list := s.List()
	if len(list) == 0 {
		t.Error("expected some entries after concurrent writes")
	}
}
