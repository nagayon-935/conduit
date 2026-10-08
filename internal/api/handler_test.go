package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/config"
	"github.com/nagayon-935/conduit/internal/control"
	"github.com/nagayon-935/conduit/internal/session"
	"github.com/nagayon-935/conduit/internal/sshconn"
	"golang.org/x/crypto/ssh"
)

type testVault struct{}

func (testVault) SignPublicKey(context.Context, string, string) (string, error) {
	return "certificate", nil
}

type testDialer struct {
	fn func(context.Context, sshconn.ConnectRequest) (*ssh.Client, *ssh.Session, io.WriteCloser, io.Reader, error)
}

func (d testDialer) Dial(ctx context.Context, r sshconn.ConnectRequest) (*ssh.Client, *ssh.Session, io.WriteCloser, io.Reader, error) {
	return d.fn(ctx, r)
}

type authClient struct {
	User        control.User
	Token, CSRF string
}
type fixture struct {
	H                 *Handler
	Store             *control.Store
	Routes            http.Handler
	Admin, Alice, Bob authClient
	Target            control.Target
	Account           control.Account
	T                 *testing.T
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store, e := control.Open(filepath.Join(t.TempDir(), "web.db"))
	if e != nil {
		t.Fatal(e)
	}
	cfg := &config.Config{DevHTTP: true, PublicURL: "http://conduit.test", AllowedCIDRs: []string{"127.0.0.0/8"}, GracePeriod: 15 * time.Minute, SessionGCInterval: time.Minute, RecordingDir: filepath.Join(t.TempDir(), "recordings")}
	f := &fixture{Store: store, T: t}
	hash, e := control.HashPassword("test-password-123")
	if e != nil {
		t.Fatal(e)
	}
	create := func(id, role string) authClient {
		u := control.User{ID: id, Login: id, Name: id, Role: role, Enabled: true, Hash: hash}
		if e := store.SaveUser("offline", u); e != nil {
			t.Fatal(e)
		}
		u, e := store.User(id)
		if e != nil {
			t.Fatal(e)
		}
		token, csrf, e := store.NewLogin(u)
		if e != nil {
			t.Fatal(e)
		}
		return authClient{u, token, csrf}
	}
	f.Admin = create("admin", "admin")
	f.Alice = create("alice", "user")
	f.Bob = create("bob", "user")
	f.Target = control.Target{ID: "target", Name: "Lab", Host: "127.0.0.1", Port: 22, Enabled: true}
	f.Account = control.Account{ID: "account", TargetID: f.Target.ID, Username: "ubuntu", AuthType: "password", Enabled: true, Sharing: true, Recording: true}
	for _, v := range []struct {
		Kind, ID, Parent string
		V                any
	}{{"target", f.Target.ID, "", f.Target}, {"account", f.Account.ID, f.Target.ID, f.Account}, {"grant", "alice:account", "alice", control.Grant{UserID: "alice", AccountID: "account", Connect: true}}, {"grant", "bob:account", "bob", control.Grant{UserID: "bob", AccountID: "account", Connect: true, View: true}}} {
		if e = store.Save("admin", v.Kind, v.ID, v.Parent, v.V); e != nil {
			t.Fatal(e)
		}
	}
	dialer := testDialer{func(ctx context.Context, r sshconn.ConnectRequest) (*ssh.Client, *ssh.Session, io.WriteCloser, io.Reader, error) {
		reader, writer := io.Pipe()
		t.Cleanup(func() { reader.Close(); writer.Close() })
		return nil, nil, writer, reader, nil
	}}
	f.H = NewHandler(cfg, session.NewManager(cfg), testVault{}, dialer, store)
	f.Routes = f.H.Routes()
	t.Cleanup(func() { f.H.Close(); store.Close() })
	return f
}
func (f *fixture) request(client *authClient, method, path string, body any) *httptest.ResponseRecorder {
	f.T.Helper()
	data := []byte{}
	if body != nil {
		data, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, "http://conduit.test"+path, bytes.NewReader(data))
	r.Header.Set("Origin", "http://conduit.test")
	if client != nil {
		r.AddCookie(&http.Cookie{Name: f.H.cookieName(), Value: client.Token})
		r.Header.Set("X-CSRF-Token", client.CSRF)
	}
	w := httptest.NewRecorder()
	f.Routes.ServeHTTP(w, r)
	return w
}
func expectStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
	}
}
func value[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func (f *fixture) connect(client *authClient) session.SessionInfo {
	w := f.request(client, "POST", "/api/app/sessions", map[string]any{"target_account_id": "account", "credentials": map[string]string{"password": "ssh-password"}})
	expectStatus(f.T, w, 201)
	return value[session.SessionInfo](f.T, w)
}
func (f *fixture) ticket(client *authClient, path string) string {
	w := f.request(client, "POST", path+"/ws-ticket", map[string]any{})
	expectStatus(f.T, w, 201)
	return value[struct {
		Ticket string `json:"ticket"`
	}](f.T, w).Ticket
}
func (f *fixture) ws(server *httptest.Server, client *authClient, ticket string, origin bool) (*websocket.Conn, *http.Response, error) {
	headers := http.Header{}
	headers.Set("Cookie", f.H.cookieName()+"="+client.Token)
	if origin {
		headers.Set("Origin", "http://conduit.test")
	}
	return websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws?ticket="+ticket, headers)
}

func TestAuthenticationAndLegacyRoutesFailClosed(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{"/api/app/targets", "/api/app/sessions", "/api/admin/users", "/api/admin/sessions", "/ws?token=old"} {
		expectStatus(t, f.request(nil, "GET", path, nil), 401)
	}
	for _, path := range []string{"/api/connect", "/api/sessions", "/api/logs", "/api/recordings/test", "/api/sessions/test/share"} {
		expectStatus(t, f.request(nil, "GET", path, nil), 410)
	}
	expectStatus(t, f.request(nil, "GET", "/healthz", nil), 200)
	expectStatus(t, f.request(&f.Alice, "GET", "/api/admin/users", nil), 403)
	expectStatus(t, f.request(&f.Admin, "GET", "/api/admin/users", nil), 200)
}
func TestMandatoryDatabase(t *testing.T) {
	h := NewHandler(&config.Config{}, session.NewManager(&config.Config{}), testVault{}, testDialer{}, nil)
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/app/targets", nil))
	expectStatus(t, w, 503)
}
func TestCookieLoginCSRFAndPasswordRotation(t *testing.T) {
	f := newFixture(t)
	w := f.request(nil, "GET", "/api/auth/csrf", nil)
	csrf := value[map[string]string](t, w)["csrf_token"]
	login := func(origin, token, password string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://conduit.test/api/auth/login", strings.NewReader(`{"login":" ALICE ","password":"`+password+`"}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", token)
		r.AddCookie(w.Result().Cookies()[0])
		out := httptest.NewRecorder()
		f.Routes.ServeHTTP(out, r)
		return out
	}
	expectStatus(t, login("http://evil.test", csrf, "test-password-123"), 403)
	expectStatus(t, login("http://conduit.test", "wrong", "test-password-123"), 403)
	expectStatus(t, login("http://conduit.test", csrf, "wrong-password"), 401)
	ok := login("http://conduit.test", csrf, "test-password-123")
	expectStatus(t, ok, 200)
	if strings.Contains(ok.Body.String(), "Hash") || strings.Contains(ok.Body.String(), f.Alice.User.Hash) {
		t.Fatal("hash exposed")
	}
	cookie := ok.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie: %+v", cookie)
	}
	var stored string
	if e := f.Store.DB.QueryRow("SELECT hash FROM login_sessions WHERE hash=?", control.TokenHash(cookie.Value)).Scan(&stored); e != nil {
		t.Fatal(e)
	}
	if stored == cookie.Value {
		t.Fatal("plaintext token persisted")
	}
	client := authClient{User: f.Alice.User, Token: cookie.Value, CSRF: value[struct {
		CSRF string `json:"csrf_token"`
	}](t, ok).CSRF}
	s := f.connect(&client)
	rotate := f.request(&client, "POST", "/api/auth/password", map[string]string{"current_password": "test-password-123", "password": "new-password-456"})
	expectStatus(t, rotate, 200)
	expectStatus(t, f.request(&client, "GET", "/api/auth/me", nil), 401)
	expectStatus(t, f.request(&f.Alice, "GET", "/api/auth/me", nil), 401)
	if _, e := f.H.sessions.Get(s.ID); e != nil {
		t.Fatal("own password change should retain SSH for reconnect")
	}
	updated, e := f.Store.User("alice")
	if e != nil || !control.Verify("new-password-456", updated.Hash) {
		t.Fatal("password not updated")
	}
	expectStatus(t, f.request(&f.Bob, "GET", "/api/auth/me", nil), 200)
}
func TestCSRFAndIdleExpiry(t *testing.T) {
	f := newFixture(t)
	bad := f.Alice
	bad.CSRF = "incorrect"
	expectStatus(t, f.request(&bad, "POST", "/api/app/sessions", map[string]any{}), 403)
	past := time.Now().Unix() - 30
	if _, e := f.Store.DB.Exec("UPDATE login_sessions SET active=? WHERE hash=?", past, control.TokenHash(f.Alice.Token)); e != nil {
		t.Fatal(e)
	}
	expectStatus(t, f.request(&f.Alice, "GET", "/api/app/targets", nil), 200)
	var active int64
	f.Store.DB.QueryRow("SELECT active FROM login_sessions WHERE hash=?", control.TokenHash(f.Alice.Token)).Scan(&active)
	if active != past {
		t.Fatal("poll extended inactivity")
	}
	f.Store.DB.Exec("UPDATE login_sessions SET active=? WHERE hash=?", time.Now().Unix()-3601, control.TokenHash(f.Alice.Token))
	expectStatus(t, f.request(&f.Alice, "GET", "/api/auth/me", nil), 401)
}
func TestOwnerIsolationAndRegisteredEndpoints(t *testing.T) {
	f := newFixture(t)
	alice := f.connect(&f.Alice)
	bob := f.connect(&f.Bob)
	if alice.User != bob.User || alice.OwnerUserID == bob.OwnerUserID {
		t.Fatal("test must use same SSH principal and different owners")
	}
	expectStatus(t, f.request(&f.Bob, "GET", "/api/app/sessions/"+alice.ID, nil), 404)
	expectStatus(t, f.request(&f.Admin, "POST", "/api/app/sessions/"+alice.ID+"/ws-ticket", map[string]any{}), 404)
	expectStatus(t, f.request(&f.Bob, "DELETE", "/api/app/sessions/"+alice.ID, map[string]any{}), 404)
	expectStatus(t, f.request(&f.Admin, "POST", "/api/app/sessions", map[string]any{"target_account_id": "account", "credentials": map[string]string{"password": "pw"}}), 404)
	expectStatus(t, f.request(&f.Alice, "POST", "/api/app/sessions", map[string]any{"target_account_id": "account", "host": "evil", "credentials": map[string]string{"password": "pw"}}), 400)
	list := value[[]session.SessionInfo](t, f.request(&f.Alice, "GET", "/api/app/sessions", nil))
	if len(list) != 1 || list[0].ID != alice.ID {
		t.Fatalf("sessions leaked: %v", list)
	}
	logs := value[struct {
		Items []control.Log `json:"items"`
	}](t, f.request(&f.Bob, "GET", "/api/app/logs", nil))
	if len(logs.Items) != 1 || logs.Items[0].Owner != "bob" {
		t.Fatalf("logs leaked: %v", logs)
	}
	expectStatus(t, f.request(&f.Alice, "GET", "/api/app/recordings/"+bob.ID, nil), 404)
}
func TestTicketsAreCookieBoundSingleUseAndRequireOrigin(t *testing.T) {
	f := newFixture(t)
	s := f.connect(&f.Alice)
	ticket := f.ticket(&f.Alice, "/api/app/sessions/"+s.ID)
	server := httptest.NewServer(f.Routes)
	defer server.Close()
	ws, resp, e := f.ws(server, &f.Bob, ticket, true)
	if e == nil {
		ws.Close()
		t.Fatal("foreign cookie accepted")
	}
	if resp.StatusCode != 403 {
		t.Fatal(resp.Status)
	}
	_, resp, e = f.ws(server, &f.Alice, ticket, false)
	if e == nil || resp.StatusCode != 403 {
		t.Fatal("missing Origin accepted")
	}
	ws, _, e = f.ws(server, &f.Alice, ticket, true)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	_, resp, e = f.ws(server, &f.Alice, ticket, true)
	if e == nil || resp.StatusCode != 403 {
		t.Fatal("ticket replay accepted")
	}
	if e = ws.WriteMessage(websocket.BinaryMessage, []byte(`{"type":"ping"}`)); e != nil {
		t.Fatal(e)
	}
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	typ, data, e := ws.ReadMessage()
	if e != nil || typ != websocket.BinaryMessage || string(data) != `{"type":"ping"}` {
		t.Fatalf("stdin did not reach SSH: %s %v", data, e)
	}
}
func TestRevokeCancelsPendingDialAndPreventsRegistration(t *testing.T) {
	f := newFixture(t)
	started := make(chan struct{})
	var canceled atomic.Bool
	f.H.dialer = testDialer{func(ctx context.Context, r sshconn.ConnectRequest) (*ssh.Client, *ssh.Session, io.WriteCloser, io.Reader, error) {
		close(started)
		<-ctx.Done()
		canceled.Store(true)
		return nil, nil, nil, nil, ctx.Err()
	}}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- f.request(&f.Alice, "POST", "/api/app/sessions", map[string]any{"target_account_id": "account", "credentials": map[string]string{"password": "pw"}})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("dial not started")
	}
	expectStatus(t, f.request(&f.Admin, "DELETE", "/api/admin/users/alice/access-grants/account", nil), 204)
	select {
	case w := <-done:
		if w.Code == 201 {
			t.Fatal("revoked dial registered")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("revocation did not interrupt setup")
	}
	if !canceled.Load() || len(f.H.sessions.List()) != 0 {
		t.Fatal("revoked session survived")
	}
}
func TestAdminRevocationAndLastAdministrator(t *testing.T) {
	f := newFixture(t)
	s := f.connect(&f.Alice)
	expectStatus(t, f.request(&f.Admin, "DELETE", "/api/admin/sessions/"+s.ID, map[string]any{}), 400)
	expectStatus(t, f.request(&f.Admin, "PATCH", "/api/admin/users/admin", map[string]any{"enabled": false}), 400)
	expectStatus(t, f.request(&f.Admin, "DELETE", "/api/admin/users/alice/access-grants/account", nil), 204)
	if _, e := f.H.sessions.Get(s.ID); e == nil {
		t.Fatal("SSH survived grant revocation")
	}
	audit, e := f.Store.Audit(0)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, v := range audit {
		if v.Action == "grant.delete" && v.Actor == "admin" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing transactional audit")
	}
}
func TestSharesRestrictRecipientsAndRevocationClosesViewer(t *testing.T) {
	f := newFixture(t)
	s := f.connect(&f.Alice)
	w := f.request(&f.Alice, "POST", "/api/app/sessions/"+s.ID+"/shares", map[string]any{"recipient_user_ids": []string{"bob"}})
	expectStatus(t, w, 201)
	v := value[struct {
		Share control.Share `json:"share"`
	}](t, w).Share
	expectStatus(t, f.request(&f.Admin, "GET", "/api/app/shared/"+v.ID, nil), 404)
	expectStatus(t, f.request(nil, "GET", "/api/app/shared/"+v.ID, nil), 401)
	ticket := f.ticket(&f.Bob, "/api/app/shared/"+v.ID)
	server := httptest.NewServer(f.Routes)
	defer server.Close()
	viewer, _, e := f.ws(server, &f.Bob, ticket, true)
	if e != nil {
		t.Fatal(e)
	}
	defer viewer.Close()
	ownerTicket := f.ticket(&f.Alice, "/api/app/sessions/"+s.ID)
	owner, _, e := f.ws(server, &f.Alice, ownerTicket, true)
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	viewer.WriteMessage(websocket.BinaryMessage, []byte("viewer must not write"))
	owner.WriteMessage(websocket.BinaryMessage, []byte("owner input"))
	owner.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, e := owner.ReadMessage()
	if e != nil || string(data) != "owner input" {
		t.Fatalf("viewer injected stdin: %s %v", data, e)
	}
	viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, e = viewer.ReadMessage()
	if e != nil || string(data) != "owner input" {
		t.Fatalf("viewer missing output %s %v", data, e)
	}
	expectStatus(t, f.request(&f.Alice, "DELETE", "/api/app/sessions/"+s.ID+"/shares/"+v.ID, map[string]any{}), 204)
	viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, e = viewer.ReadMessage(); e == nil {
		t.Fatal("viewer socket survived revocation")
	}
	if _, e = f.H.sessions.Get(s.ID); e != nil {
		t.Fatal("viewer revoke ended owner's SSH")
	}
}
func TestDisabledUserInvalidatesLoginsAndSSH(t *testing.T) {
	f := newFixture(t)
	s := f.connect(&f.Alice)
	expectStatus(t, f.request(&f.Admin, "PATCH", "/api/admin/users/alice", map[string]any{"enabled": false}), 200)
	expectStatus(t, f.request(&f.Alice, "GET", "/api/auth/me", nil), 401)
	if _, e := f.H.sessions.Get(s.ID); e == nil {
		t.Fatal("disabled user retained SSH")
	}
}
func TestNetworkPolicy(t *testing.T) {
	f := newFixture(t)
	for _, host := range []string{"169.254.169.254", "0.0.0.0", "10.1.2.3", "::1"} {
		if _, e := f.H.pin(context.Background(), host); e == nil {
			t.Fatalf("denied address accepted %s", host)
		}
	}
	if ip, e := f.H.pin(context.Background(), "127.0.0.1"); e != nil || ip != "127.0.0.1" {
		t.Fatal(ip, e)
	}
	f.H.config.DevHTTP = false
	if _, e := f.H.pin(context.Background(), "127.0.0.1"); e == nil {
		t.Fatal("production loopback accepted")
	}
}
func TestDialFailuresPersistOwnedLog(t *testing.T) {
	f := newFixture(t)
	f.H.dialer = testDialer{func(context.Context, sshconn.ConnectRequest) (*ssh.Client, *ssh.Session, io.WriteCloser, io.Reader, error) {
		return nil, nil, nil, nil, errors.New("unable to authenticate private secret")
	}}
	w := f.request(&f.Alice, "POST", "/api/app/sessions", map[string]any{"target_account_id": "account", "credentials": map[string]string{"password": "pw"}})
	expectStatus(t, w, 502)
	if strings.Contains(w.Body.String(), "private secret") {
		t.Fatal("dial detail leaked")
	}
	logs, e := f.Store.Logs("alice", "")
	if e != nil || len(logs) != 1 || logs[0].Reason != "ssh_dial_failed" {
		t.Fatal(logs, e)
	}
}

func TestRegisteredJumpPermitsTransitWithoutDirectGrant(t *testing.T) {
	f := newFixture(t)
	jt := control.Target{ID: "jump-target", Name: "Jump", Host: "127.0.0.1", Port: 2222, Enabled: true}
	ja := control.Account{ID: "jump-account", TargetID: jt.ID, Username: "jump-user", AuthType: "password", Enabled: true}
	if e := f.Store.Save("admin", "target", jt.ID, "", jt); e != nil {
		t.Fatal(e)
	}
	if e := f.Store.Save("admin", "account", ja.ID, jt.ID, ja); e != nil {
		t.Fatal(e)
	}
	target := f.Target
	target.JumpAccountID = ja.ID
	if e := f.Store.Save("admin", "target", target.ID, "", target); e != nil {
		t.Fatal(e)
	}
	var captured sshconn.ConnectRequest
	f.H.dialer = testDialer{func(_ context.Context, r sshconn.ConnectRequest) (*ssh.Client, *ssh.Session, io.WriteCloser, io.Reader, error) {
		captured = r
		reader, writer := io.Pipe()
		t.Cleanup(func() { reader.Close(); writer.Close() })
		return nil, nil, writer, reader, nil
	}}
	w := f.request(&f.Alice, "POST", "/api/app/sessions", map[string]any{"target_account_id": f.Account.ID, "credentials": map[string]string{"password": "target-password"}, "jump_credentials": map[string]string{"password": "jump-password"}})
	expectStatus(t, w, 201)
	if captured.JumpUser != "jump-user" || captured.JumpHost != "127.0.0.1" || captured.JumpDialIP != "127.0.0.1" {
		t.Fatalf("jump not pinned: %+v", captured)
	}
	expectStatus(t, f.request(&f.Alice, "POST", "/api/app/sessions", map[string]any{"target_account_id": ja.ID, "credentials": map[string]string{"password": "jump-password"}}), 404)
	expectStatus(t, f.request(&f.Admin, "PATCH", "/api/admin/targets/"+jt.ID, map[string]any{"jump_account_id": f.Account.ID}), 400)
}
