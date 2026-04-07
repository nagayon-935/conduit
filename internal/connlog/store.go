package connlog

import (
	"sync"
	"time"
)

type Entry struct {
	ID             string     `json:"id"`
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	User           string     `json:"user"`
	ConnectedAt    time.Time  `json:"connected_at"`
	DisconnectedAt *time.Time `json:"disconnected_at,omitempty"`
	// Error is set when the connection attempt failed or terminated abnormally.
	Error string `json:"error,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	entries []*Entry
	maxSize int
}

func NewStore(maxSize int) *Store {
	return &Store{maxSize: maxSize, entries: make([]*Entry, 0)}
}

func (s *Store) Add(e *Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append([]*Entry{e}, s.entries...) // newest first
	if len(s.entries) > s.maxSize {
		s.entries = s.entries[:s.maxSize]
	}
}

// UpdateError sets an error message and disconnection time on the entry
// identified by id. It is a no-op when the id is not found.
func (s *Store) UpdateError(id string, errMsg string, disconnectedAt time.Time) {
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

func (s *Store) List() []*Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Entry, len(s.entries))
	copy(result, s.entries)
	return result
}
