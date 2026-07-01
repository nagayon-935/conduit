package connlog

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS connection_logs (
	id              TEXT PRIMARY KEY,
	host            TEXT NOT NULL,
	port            INTEGER NOT NULL,
	user            TEXT NOT NULL,
	connected_at    INTEGER NOT NULL,
	disconnected_at INTEGER,
	error           TEXT NOT NULL DEFAULT '',
	recording_path  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_connected_at ON connection_logs(connected_at DESC);
`

// SQLiteStore is a SQLite-backed Store.
type SQLiteStore struct {
	db       *sql.DB
	maxRows  int
	syncTrim bool // If true, trim is executed synchronously. Used primarily for tests.
}

// NewSQLiteStore opens (or creates) the SQLite database at path and returns a Store.
func NewSQLiteStore(path string, maxRows int) (*SQLiteStore, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("connlog: open sqlite: %w", err)
	}
	// Single writer to avoid SQLITE_BUSY under concurrent writes.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connlog: apply schema: %w", err)
	}

	return &SQLiteStore{db: db, maxRows: maxRows, syncTrim: false}, nil
}

// Close releases the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Add(e *Entry) {
	var disconnectedAt *int64
	if e.DisconnectedAt != nil {
		t := e.DisconnectedAt.UnixMilli()
		disconnectedAt = &t
	}

	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO connection_logs
			(id, host, port, user, connected_at, disconnected_at, error, recording_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Host, e.Port, e.User,
		e.ConnectedAt.UnixMilli(), disconnectedAt,
		e.Error, e.RecordingPath,
	)
	if err != nil {
		slog.Error("connlog: sqlite Add failed", "error", err)
		return
	}

	if s.syncTrim {
		s.trim()
	} else {
		go s.trim()
	}
}

func (s *SQLiteStore) UpdateError(id, errMsg string, disconnectedAt time.Time) {
	_, err := s.db.Exec(
		`UPDATE connection_logs SET error = ?, disconnected_at = ? WHERE id = ?`,
		errMsg, disconnectedAt.UnixMilli(), id,
	)
	if err != nil {
		slog.Error("connlog: sqlite UpdateError failed", "error", err)
	}
}

func (s *SQLiteStore) UpdateDisconnected(id string, disconnectedAt time.Time) {
	_, err := s.db.Exec(
		`UPDATE connection_logs SET disconnected_at = ? WHERE id = ?`,
		disconnectedAt.UnixMilli(), id,
	)
	if err != nil {
		slog.Error("connlog: sqlite UpdateDisconnected failed", "error", err)
	}
}

func (s *SQLiteStore) List() []*Entry {
	rows, err := s.db.Query(
		`SELECT id, host, port, user, connected_at, disconnected_at, error, recording_path
		 FROM connection_logs
		 ORDER BY connected_at DESC
		 LIMIT ?`,
		s.maxRows,
	)
	if err != nil {
		slog.Error("connlog: sqlite List failed", "error", err)
		return nil
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			slog.Error("connlog: sqlite scan failed", "error", err)
			continue
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []*Entry{}
	}
	return entries
}

func scanEntry(rows *sql.Rows) (*Entry, error) {
	var (
		e              Entry
		connectedAtMs  int64
		disconnectedMs *int64
	)
	err := rows.Scan(
		&e.ID, &e.Host, &e.Port, &e.User,
		&connectedAtMs, &disconnectedMs,
		&e.Error, &e.RecordingPath,
	)
	if err != nil {
		return nil, err
	}
	e.ConnectedAt = time.UnixMilli(connectedAtMs).UTC()
	if disconnectedMs != nil {
		t := time.UnixMilli(*disconnectedMs).UTC()
		e.DisconnectedAt = &t
	}
	return &e, nil
}

// trim removes rows beyond maxRows, keeping the most recent ones.
func (s *SQLiteStore) trim() {
	_, err := s.db.Exec(
		`DELETE FROM connection_logs
		 WHERE id NOT IN (
			 SELECT id FROM connection_logs ORDER BY connected_at DESC LIMIT ?
		 )`,
		s.maxRows,
	)
	if err != nil {
		slog.Error("connlog: sqlite trim failed", "error", err)
	}
}
