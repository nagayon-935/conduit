package connlog

import (
	"testing"
	"time"
)

func TestMemoryStore_UpdateError(t *testing.T) {
	now := time.Now()
	disconnected := now.Add(time.Minute)

	tests := []struct {
		name       string
		updateID   string
		wantErrMsg string
		wantClosed bool
	}{
		{"existing entry gets error and disconnect time", "1", "boom", true},
		{"unknown id is a no-op", "missing", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewMemoryStore(5)
			s.Add(&Entry{ID: "1", Host: "h", Port: 22, User: "u", ConnectedAt: now})

			s.UpdateError(tt.updateID, "boom", disconnected)

			got := s.List()[0]
			if got.Error != tt.wantErrMsg {
				t.Errorf("Error = %q, want %q", got.Error, tt.wantErrMsg)
			}
			if tt.wantClosed && (got.DisconnectedAt == nil || !got.DisconnectedAt.Equal(disconnected)) {
				t.Errorf("DisconnectedAt = %v, want %v", got.DisconnectedAt, disconnected)
			}
			if !tt.wantClosed && got.DisconnectedAt != nil {
				t.Errorf("DisconnectedAt = %v, want nil", got.DisconnectedAt)
			}
		})
	}
}

func TestMemoryStore_UpdateDisconnected(t *testing.T) {
	now := time.Now()
	disconnected := now.Add(time.Minute)

	t.Run("existing entry gets disconnect time", func(t *testing.T) {
		s := NewMemoryStore(5)
		s.Add(&Entry{ID: "1", Host: "h", Port: 22, User: "u", ConnectedAt: now})

		s.UpdateDisconnected("1", disconnected)

		got := s.List()[0]
		if got.DisconnectedAt == nil || !got.DisconnectedAt.Equal(disconnected) {
			t.Errorf("DisconnectedAt = %v, want %v", got.DisconnectedAt, disconnected)
		}
		if got.Error != "" {
			t.Errorf("Error = %q, want empty", got.Error)
		}
	})

	t.Run("unknown id is a no-op", func(t *testing.T) {
		s := NewMemoryStore(5)
		s.Add(&Entry{ID: "1", Host: "h", Port: 22, User: "u", ConnectedAt: now})

		s.UpdateDisconnected("missing", disconnected)

		if got := s.List()[0]; got.DisconnectedAt != nil {
			t.Errorf("DisconnectedAt = %v, want nil", got.DisconnectedAt)
		}
	})
}

// TestMemoryStore_ListIsSnapshot verifies List returns a copy: mutating the
// returned slice must not affect subsequent reads.
func TestMemoryStore_ListIsSnapshot(t *testing.T) {
	s := NewMemoryStore(5)
	s.Add(&Entry{ID: "1", ConnectedAt: time.Now()})

	first := s.List()
	first[0] = nil

	if second := s.List(); second[0] == nil || second[0].ID != "1" {
		t.Errorf("List() was mutated through a returned slice")
	}
}
