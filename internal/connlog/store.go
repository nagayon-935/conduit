package connlog

import (
	"sync"
	"time"
)

// Store is the interface for connection log persistence.
type Store interface {
	Add(e *Entry)
	UpdateError(id, errMsg string, disconnectedAt time.Time)
	UpdateDisconnected(id string, disconnectedAt time.Time)
	List() []*Entry
}

// Entry represents a single connection log record.
type Entry struct {
	ID             string     `json:"id"`
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	User           string     `json:"user"`
	ConnectedAt    time.Time  `json:"connected_at"`
	DisconnectedAt *time.Time `json:"disconnected_at,omitempty"`
	// Error is set when the connection attempt failed or terminated abnormally.
	Error string `json:"error,omitempty"`
	// RecordingPath is the path to the asciinema recording file, if recorded.
	RecordingPath string `json:"recording_path,omitempty"`
}

// MemoryStore is an in-memory Store (used when no DB path is configured).
type MemoryStore struct {
	mu      sync.RWMutex
	entries []*Entry
	maxSize int
}

// NewMemoryStore creates an in-memory Store capped at maxSize entries.
func NewMemoryStore(maxSize int) *MemoryStore {
	return &MemoryStore{maxSize: maxSize, entries: make([]*Entry, 0)}
}

func (s *MemoryStore) Add(e *Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append([]*Entry{e}, s.entries...) // newest first
	if len(s.entries) > s.maxSize {
		s.entries = s.entries[:s.maxSize]
	}
}

// UpdateError sets an error message and disconnection time on the entry
// identified by id. It is a no-op when the id is not found.
func (s *MemoryStore) UpdateError(id string, errMsg string, disconnectedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			t := disconnectedAt
			e.DisconnectedAt = &t
			e.Error = errMsg
			return
		}
	}
}

// UpdateDisconnected sets the disconnection time on the entry identified by id.
func (s *MemoryStore) UpdateDisconnected(id string, disconnectedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			t := disconnectedAt
			e.DisconnectedAt = &t
			return
		}
	}
}

func (s *MemoryStore) List() []*Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Entry, len(s.entries))
	copy(result, s.entries)
	return result
}
