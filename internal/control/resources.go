package control

import (
	"database/sql"
	"encoding/json"
)

func Put(tx *sql.Tx, kind, id, parent string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO control_objects VALUES(?,?,?,?) ON CONFLICT(kind,id) DO UPDATE SET parent=excluded.parent,body=excluded.body", kind, id, parent, string(b))
	return err
}

func (s *Store) Save(actor, kind, id, parent string, v any) error {
	return s.Mutation(actor, kind+".save", id, "", func(tx *sql.Tx) error { return Put(tx, kind, id, parent, v) })
}

func (s *Store) Get(kind, id string, out any) error {
	var b string
	if err := s.DB.QueryRow("SELECT body FROM control_objects WHERE kind=? AND id=?", kind, id).Scan(&b); err != nil {
		return err
	}
	return json.Unmarshal([]byte(b), out)
}

func (s *Store) Objects(kind, parent string) ([]json.RawMessage, error) {
	q := "SELECT body FROM control_objects WHERE kind=?"
	args := []any{kind}
	if parent != "" {
		q += " AND parent=?"
		args = append(args, parent)
	}
	q += " ORDER BY id"
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b string
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}

func (s *Store) Delete(actor, kind, id, reason string) error {
	return s.Mutation(actor, kind+".delete", id, reason, func(tx *sql.Tx) error {
		_, e := tx.Exec("DELETE FROM control_objects WHERE kind=? AND id=?", kind, id)
		return e
	})
}

func (s *Store) Grant(uid, aid string) (Grant, error) {
	var g Grant
	err := s.Get("grant", uid+":"+aid, &g)
	return g, err
}

func (s *Store) Share(id string) (Share, error) {
	var persisted struct {
		Share
		Hash string `json:"creator_login_hash"`
	}
	err := s.Get("share", id, &persisted)
	persisted.Share.LoginHash = persisted.Hash
	return persisted.Share, err
}

func (s *Store) SaveShare(actor string, v Share) error {
	return s.Save(actor, "share", v.ID, v.SessionID, struct {
		Share
		Hash string `json:"creator_login_hash"`
	}{v, v.LoginHash})
}
