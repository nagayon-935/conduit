package control

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrationLockAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conduit.db")
	legacy, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = legacy.Exec(`CREATE TABLE connection_logs(id TEXT PRIMARY KEY,host TEXT,port INTEGER,user TEXT,connected_at INTEGER,disconnected_at INTEGER,error TEXT,recording_path TEXT); INSERT INTO connection_logs VALUES('legacy','host',22,'ubuntu',1700000000000,1700000001000,'','')`)
	if e != nil {
		t.Fatal(e)
	}
	legacy.Close()
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	l, e := s.Log("legacy", "")
	if e != nil || l.Owner != "" || l.Started != 1700000000 || *l.Ended != 1700000001 {
		t.Fatalf("legacy migration %+v %v", l, e)
	}
	if _, e = s.Log("legacy", "alice"); e != sql.ErrNoRows {
		t.Fatal("legacy log assigned to user")
	}
	if e = s.Bootstrap(" ADMIN ", "temporary-password"); e != nil {
		t.Fatal(e)
	}
	users, _ := s.Users()
	token, _, e := s.NewLogin(users[0])
	if e != nil {
		t.Fatal(e)
	}
	if other, e := Open(path); e == nil {
		other.Close()
		t.Fatal("second process acquired database")
	}
	if _, _, e = s.Login(TokenHash(token)); e != nil {
		t.Fatal("failed second open invalidated login")
	}
	if e = s.AddLog(Log{ID: "running", Owner: users[0].ID, Host: "test", Port: 22, Username: "ubuntu", Started: time.Now().Unix()}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("DELETE FROM ssh_session_records WHERE id='legacy'"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, _, e = s.Login(TokenHash(token)); e != sql.ErrNoRows {
		t.Fatal("old login survived restart")
	}
	if _, e = s.Log("legacy", ""); e != sql.ErrNoRows {
		t.Fatal("deleted legacy log reimported")
	}
	l, e = s.Log("running", "")
	if e != nil || l.Ended == nil || l.Reason != "server_restart" {
		t.Fatal(l, e)
	}
	if e = s.Bootstrap("other", "temporary-password"); e == nil {
		t.Fatal("bootstrap repeated")
	}
	stat, e := os.Stat(path)
	if e != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("unsafe DB permissions", e)
	}
}
func TestMutationRollsBackWithoutAudit(t *testing.T) {
	s, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Save("admin", "target", "existing", "", map[string]string{"name": "original"}); e != nil {
		t.Fatal(e)
	}
	before, _ := s.Audit(0)
	e = s.Mutation("admin", "target.save", "existing", "", func(tx *sql.Tx) error {
		if e := Put(tx, "target", "existing", "", map[string]string{"name": "replacement"}); e != nil {
			return e
		}
		return errors.New("reject")
	})
	if e == nil {
		t.Fatal("mutation not rejected")
	}
	var target map[string]string
	if e = s.Get("target", "existing", &target); e != nil || target["name"] != "original" {
		t.Fatal("partial mutation", target, e)
	}
	after, _ := s.Audit(0)
	if len(after) != len(before) {
		t.Fatal("failed mutation added success audit")
	}
}
func TestPasswordHashesAndAuthVersion(t *testing.T) {
	h1, e := HashPassword("safe-password-123")
	if e != nil {
		t.Fatal(e)
	}
	h2, _ := HashPassword("safe-password-123")
	if h1 == h2 || strings.Contains(h1, "safe-password") || !Verify("safe-password-123", h1) || Verify("wrong", h1) {
		t.Fatal("password hash incorrect")
	}
	s, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.SaveUser("offline", User{ID: "admin", Login: " ADMIN ", Name: "Admin", Role: "admin", Enabled: true, Hash: h1}); e != nil {
		t.Fatal(e)
	}
	u, _ := s.User("admin")
	token, _, _ := s.NewLogin(u)
	u.Name = "New Name"
	if e = s.SaveUser("admin", u); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Login(TokenHash(token)); e != nil {
		t.Fatal("name change invalidated login")
	}
	u, _ = s.User("admin")
	u.Role = "user"
	if e = s.SaveUser("admin", u); e == nil {
		t.Fatal("last admin demoted")
	}
}
