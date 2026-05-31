package connlog

import (
	"testing"
	"time"
)

func TestStore_AddAndList(t *testing.T) {
	maxSize := 3
	store := NewMemoryStore(maxSize)

	entries := []*Entry{
		{ID: "1", Host: "host1", Port: 22, User: "user1", ConnectedAt: time.Now()},
		{ID: "2", Host: "host2", Port: 22, User: "user2", ConnectedAt: time.Now()},
		{ID: "3", Host: "host3", Port: 22, User: "user3", ConnectedAt: time.Now()},
		{ID: "4", Host: "host4", Port: 22, User: "user4", ConnectedAt: time.Now()},
	}

	// Add entries one by one
	for _, e := range entries {
		store.Add(e)
	}

	// List entries
	list := store.List()

	// Check size (should be capped at maxSize)
	if len(list) != maxSize {
		t.Errorf("expected list size %d, got %d", maxSize, len(list))
	}

	// Check order (newest first)
	if list[0].ID != "4" {
		t.Errorf("expected newest entry ID '4', got '%s'", list[0].ID)
	}
	if list[2].ID != "2" {
		t.Errorf("expected oldest entry ID '2', got '%s'", list[2].ID)
	}
}

func TestStore_Empty(t *testing.T) {
	store := NewMemoryStore(5)
	list := store.List()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}
