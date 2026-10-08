package control

import (
	"database/sql"
	"errors"
	"time"
)

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Login, &u.Name, &u.Role, &u.Enabled, &u.MustChange, &u.Version, &u.Hash)
	return u, err
}

func (s *Store) User(id string) (User, error) {
	return scanUser(s.DB.QueryRow("SELECT * FROM web_users WHERE id=?", id))
}

var ErrCredentials = errors.New("invalid login or password")

var ErrLoginExpired = errors.New("login expired")

func (s *Store) Authenticate(login, password string) (User, error) {
	u, err := scanUser(s.DB.QueryRow("SELECT * FROM web_users WHERE login=?", Normalize(login)))
	if err != nil && err != sql.ErrNoRows {
		return User{}, err
	}
	hash := u.Hash
	if err != nil {
		hash = s.dummy
	}
	ok := Verify(password, hash)
	if !ok || err != nil || !u.Enabled {
		return User{}, ErrCredentials
	}
	return u, nil
}

func (s *Store) Users() ([]User, error) {
	rows, err := s.DB.Query("SELECT * FROM web_users ORDER BY login")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, e := scanUser(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) SaveUser(actor string, u User) error {
	if u.ID == "" {
		u.ID = Random()
	}
	u.Login = Normalize(u.Login)
	if u.Login == "" || len(u.Login) > 128 || len(u.Name) > 256 || (u.Role != "admin" && u.Role != "user") {
		return errors.New("invalid user")
	}
	return s.Mutation(actor, "user.save", u.ID, "", func(tx *sql.Tx) error {
		var oldRole, oldHash string
		var oldEnabled, oldMustChange bool
		err := tx.QueryRow("SELECT role,enabled,hash,must_change FROM web_users WHERE id=?", u.ID).Scan(&oldRole, &oldEnabled, &oldHash, &oldMustChange)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if oldRole == "admin" && oldEnabled && (!u.Enabled || u.Role != "admin") {
			var n int
			if err = tx.QueryRow("SELECT count(*) FROM web_users WHERE role='admin' AND enabled=1").Scan(&n); err != nil {
				return err
			}
			if n <= 1 {
				return errors.New("cannot disable or demote the last active administrator")
			}
		}
		if err == sql.ErrNoRows {
			if u.Hash == "" {
				return errors.New("password required")
			}
			_, err = tx.Exec("INSERT INTO web_users VALUES(?,?,?,?,?,?,1,?)", u.ID, u.Login, u.Name, u.Role, u.Enabled, u.MustChange, u.Hash)
			return err
		}
		increment := 0
		if oldRole != u.Role || oldEnabled != u.Enabled || oldMustChange != u.MustChange || (u.Hash != "" && u.Hash != oldHash) {
			increment = 1
		}
		_, err = tx.Exec("UPDATE web_users SET name=?,role=?,enabled=?,must_change=?,hash=CASE WHEN ?='' THEN hash ELSE ? END,version=version+? WHERE id=?", u.Name, u.Role, u.Enabled, u.MustChange, u.Hash, u.Hash, increment, u.ID)
		return err
	})
}

func (s *Store) Bootstrap(login, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	users, err := s.Users()
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return errors.New("already initialized; use reset-password for recovery")
	}
	return s.SaveUser("offline", User{ID: Random(), Login: login, Name: login, Role: "admin", Enabled: true, MustChange: true, Hash: hash})
}

func (s *Store) NewLogin(u User) (string, string, error) {
	token, csrf := Random(), Random()
	now := time.Now().Unix()
	err := s.Mutation(u.ID, "auth.login", u.ID, "", func(tx *sql.Tx) error {
		_, e := tx.Exec("INSERT INTO login_sessions VALUES(?,?,?,?,?,?)", TokenHash(token), u.ID, u.Version, csrf, now, now)
		return e
	})
	return token, csrf, err
}

func (s *Store) Login(hash string) (Login, User, error) {
	var l Login
	err := s.DB.QueryRow("SELECT * FROM login_sessions WHERE hash=?", hash).Scan(&l.Hash, &l.UserID, &l.Version, &l.CSRF, &l.Created, &l.Active)
	if err != nil {
		return l, User{}, err
	}
	u, err := s.User(l.UserID)
	now := time.Now().Unix()
	if err != nil {
		return l, User{}, err
	}
	p, e := s.Policy()
	if e != nil {
		return l, User{}, e
	}
	if !u.Enabled || u.Version != l.Version || now >= l.Created+int64(p.LoginHours)*3600 || now >= l.Active+int64(p.LoginIdleMinutes)*60 {
		return l, User{}, ErrLoginExpired
	}
	return l, u, nil
}

func (s *Store) Touch(hash string) error {
	_, err := s.DB.Exec("UPDATE login_sessions SET active=? WHERE hash=?", time.Now().Unix(), hash)
	return err
}

func (s *Store) Revoke(hash string) error {
	var uid string
	if err := s.DB.QueryRow("SELECT user_id FROM login_sessions WHERE hash=?", hash).Scan(&uid); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	return s.Mutation(uid, "auth.logout", uid, "", func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM login_sessions WHERE hash=?", hash)
		return err
	})
}

func (s *Store) RevokeUser(id string) error {
	_, err := s.DB.Exec("DELETE FROM login_sessions WHERE user_id=?", id)
	return err
}
