package control

import (
	"database/sql"
	"time"
)

func (s *Store) AddLog(l Log) error {
	_, err := s.DB.Exec("INSERT INTO ssh_session_records VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", l.ID, l.Owner, l.AccountID, l.SessionID, l.Host, l.Port, l.Username, l.Started, l.Ended, l.Error, l.Reason, l.Path)
	return err
}

func (s *Store) EndLog(id, reason string) error {
	_, err := s.DB.Exec("UPDATE ssh_session_records SET ended=?,reason=? WHERE id=? AND ended IS NULL", time.Now().Unix(), reason, id)
	return err
}

func scanLog(r interface{ Scan(...any) error }) (Log, error) {
	var l Log
	var owner, acct, sess sql.NullString
	err := r.Scan(&l.ID, &owner, &acct, &sess, &l.Host, &l.Port, &l.Username, &l.Started, &l.Ended, &l.Error, &l.Reason, &l.Path)
	l.Owner, l.AccountID, l.SessionID = owner.String, acct.String, sess.String
	l.Recording = l.Path != ""
	return l, err
}

func (s *Store) Log(id, owner string) (Log, error) {
	q := "SELECT * FROM ssh_session_records WHERE id=?"
	args := []any{id}
	if owner != "" {
		q += " AND owner=?"
		args = append(args, owner)
	}
	return scanLog(s.DB.QueryRow(q, args...))
}

func (s *Store) Logs(owner, cursor string) ([]Log, error) {
	q := "SELECT * FROM ssh_session_records WHERE 1=1"
	args := []any{}
	if owner != "" {
		q += " AND owner=?"
		args = append(args, owner)
	}
	if cursor != "" {
		q += " AND (started < (SELECT started FROM ssh_session_records WHERE id=?) OR (started=(SELECT started FROM ssh_session_records WHERE id=?) AND id<?))"
		args = append(args, cursor, cursor, cursor)
	}
	q += " ORDER BY started DESC,id DESC LIMIT 50"
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Log{}
	for rows.Next() {
		l, e := scanLog(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) Audit(cursor int64) ([]Audit, error) {
	rows, err := s.DB.Query("SELECT * FROM audit_events WHERE (?=0 OR id<?) ORDER BY id DESC LIMIT 50", cursor, cursor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Audit{}
	for rows.Next() {
		var a Audit
		if err = rows.Scan(&a.ID, &a.Actor, &a.Action, &a.Resource, &a.Reason, &a.At); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
