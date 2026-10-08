package control

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB    *sql.DB
	dummy string
	lock  *os.File
}

const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY);
CREATE TABLE IF NOT EXISTS web_users(id TEXT PRIMARY KEY,login TEXT UNIQUE NOT NULL,name TEXT NOT NULL,role TEXT NOT NULL,enabled INTEGER NOT NULL,must_change INTEGER NOT NULL,version INTEGER NOT NULL,hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS login_sessions(hash TEXT PRIMARY KEY,user_id TEXT NOT NULL,version INTEGER NOT NULL,csrf TEXT NOT NULL,created INTEGER NOT NULL,active INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS login_user ON login_sessions(user_id);
CREATE TABLE IF NOT EXISTS control_objects(kind TEXT NOT NULL,id TEXT NOT NULL,parent TEXT NOT NULL,body TEXT NOT NULL,PRIMARY KEY(kind,id));
CREATE INDEX IF NOT EXISTS object_parent ON control_objects(kind,parent);
CREATE TABLE IF NOT EXISTS ssh_session_records(id TEXT PRIMARY KEY,owner TEXT,account TEXT,session TEXT,host TEXT NOT NULL,port INTEGER NOT NULL,username TEXT NOT NULL,started INTEGER NOT NULL,ended INTEGER,error TEXT NOT NULL,reason TEXT NOT NULL,path TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS session_owner ON ssh_session_records(owner,started DESC,id DESC);
CREATE TABLE IF NOT EXISTS audit_events(id INTEGER PRIMARY KEY AUTOINCREMENT,actor TEXT NOT NULL,action TEXT NOT NULL,resource TEXT NOT NULL,reason TEXT NOT NULL,at INTEGER NOT NULL);
INSERT OR IGNORE INTO schema_migrations VALUES(1);
`

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("DB_PATH is required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		f.Close()
		if err = os.Chmod(path, 0600); err != nil {
			return nil, err
		}
	}
	var lock *os.File
	var err error
	if path != ":memory:" {
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return nil, err
		}
		lock, err = lockDatabase(path + ".lock")
		if err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		if lock != nil {
			lock.Close()
		}
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Store, error) {
		db.Close()
		if lock != nil {
			lock.Close()
		}
		return nil, err
	}
	if _, err = db.Exec("PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL;" + schema); err != nil {
		return fail(err)
	}
	s := &Store{DB: db, lock: lock}
	s.dummy, err = HashPassword("dummy-password-for-timing")
	if err != nil {
		return fail(err)
	}
	// Import pre-authentication logs once, retaining their unknown owner.
	var migrated int
	if err = db.QueryRow("SELECT count(*) FROM schema_migrations WHERE version=2").Scan(&migrated); err != nil {
		return fail(err)
	}
	if migrated == 0 {
		tx, e := db.Begin()
		if e != nil {
			return fail(e)
		}
		var legacy int
		e = tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='connection_logs'").Scan(&legacy)
		if e == nil && legacy > 0 {
			_, e = tx.Exec(`INSERT OR IGNORE INTO ssh_session_records SELECT id,NULL,NULL,NULL,host,port,user,connected_at/1000,COALESCE(disconnected_at/1000,unixepoch()),error,'legacy_import',recording_path FROM connection_logs`)
		}
		if e == nil {
			_, e = tx.Exec("INSERT INTO schema_migrations VALUES(2)")
		}
		if e == nil {
			e = tx.Commit()
		} else {
			tx.Rollback()
		}
		if e != nil {
			return fail(e)
		}
	}
	if _, err = db.Exec("UPDATE ssh_session_records SET ended=unixepoch(),reason='server_restart' WHERE ended IS NULL; DELETE FROM login_sessions; DELETE FROM control_objects WHERE kind='share'"); err != nil {
		return fail(err)
	}
	return s, nil
}

func (s *Store) Close() error {
	err := s.DB.Close()
	if s.lock != nil {
		err = errors.Join(err, s.lock.Close())
		s.lock = nil
	}
	return err
}

func audit(tx *sql.Tx, actor, action, id, reason string) error {
	_, err := tx.Exec("INSERT INTO audit_events(actor,action,resource,reason,at) VALUES(?,?,?,?,?)", actor, action, id, reason, time.Now().Unix())
	return err
}

func (s *Store) Mutation(actor, action, id, reason string, fn func(*sql.Tx) error) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	if err = audit(tx, actor, action, id, reason); err != nil {
		return err
	}
	return tx.Commit()
}
