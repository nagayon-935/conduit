package api

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nagayon-935/conduit/internal/control"
)

func TestRecordingOwnershipAndRetention(t *testing.T) {
	f := newFixture(t)
	if e := os.MkdirAll(f.H.config.RecordingDir, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(f.H.config.RecordingDir, "alice.cast")
	if e := os.WriteFile(path, []byte("recording-data"), 0600); e != nil {
		t.Fatal(e)
	}
	ended := time.Now().Unix()
	l := control.Log{ID: "alice-log", Owner: "alice", Host: "test", Port: 22, Username: "ubuntu", Started: ended - 40*86400, Ended: &ended, Path: path}
	if e := f.Store.AddLog(l); e != nil {
		t.Fatal(e)
	}
	expectStatus(t, f.request(&f.Bob, "GET", "/api/app/recordings/alice-log", nil), 404)
	expectStatus(t, f.request(&f.Alice, "GET", "/api/app/recordings/alice-log", nil), 200)
	expectStatus(t, f.request(&f.Admin, "GET", "/api/admin/recordings/alice-log", nil), 200)
	logs := f.request(&f.Alice, "GET", "/api/app/logs", nil)
	if json.Valid(logs.Body.Bytes()) && string(logs.Body.Bytes()) == path {
		t.Fatal("path exposed")
	}
	if e := f.H.retain(); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatalf("expired recording retained: %v", e)
	}
	saved, e := f.Store.Log(l.ID, "alice")
	if e != nil || saved.Path != "" {
		t.Fatal("recording path not removed", e)
	}
	expectStatus(t, f.request(&f.Alice, "GET", "/api/app/recordings/alice-log", nil), 404)
}
func TestForcedPasswordChangeAndSecureCookie(t *testing.T) {
	f := newFixture(t)
	u, e := f.Store.User("alice")
	if e != nil {
		t.Fatal(e)
	}
	u.MustChange = true
	if e = f.Store.SaveUser("admin", u); e != nil {
		t.Fatal(e)
	}
	u, _ = f.Store.User("alice")
	token, csrf, e := f.Store.NewLogin(u)
	if e != nil {
		t.Fatal(e)
	}
	client := authClient{u, token, csrf}
	expectStatus(t, f.request(&client, "GET", "/api/app/targets", nil), 403)
	expectStatus(t, f.request(&client, "GET", "/api/auth/me", nil), 200)
	f.H.config.DevHTTP = false
	w := httptest.NewRecorder()
	f.H.setCookie(w, "token")
	cookie := w.Result().Cookies()[0]
	if cookie.Name != "__Host-conduit_session" || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" {
		t.Fatalf("unsafe cookie: %+v", cookie)
	}
}
func TestLogoutOnlyRevokesCurrentLogin(t *testing.T) {
	f := newFixture(t)
	s := f.connect(&f.Alice)
	token, csrf, e := f.Store.NewLogin(f.Alice.User)
	if e != nil {
		t.Fatal(e)
	}
	other := authClient{f.Alice.User, token, csrf}
	expectStatus(t, f.request(&f.Alice, "POST", "/api/auth/logout", map[string]any{}), 204)
	expectStatus(t, f.request(&f.Alice, "GET", "/api/auth/me", nil), 401)
	expectStatus(t, f.request(&other, "GET", "/api/auth/me", nil), 200)
	if _, e = f.H.sessions.Get(s.ID); e != nil {
		t.Fatal("normal logout should allow reconnect")
	}
	expectStatus(t, f.request(&other, "POST", "/api/auth/logout", map[string]bool{"terminate_sessions": true}), 204)
	if _, e = f.H.sessions.Get(s.ID); e == nil {
		t.Fatal("logout+terminate retained SSH")
	}
}

func TestPolicyRevisionCookieAndHealth(t *testing.T) {
	f := newFixture(t)
	p := f.request(&f.Admin, "PATCH", "/api/admin/settings", map[string]any{"revision": 1, "login_absolute_hours": 12, "recording_enabled": true})
	if p.Code != 200 {
		t.Fatalf("settings: %d %s", p.Code, p.Body.String())
	}
	stale := f.request(&f.Admin, "PATCH", "/api/admin/settings", map[string]any{"revision": 1, "login_absolute_hours": 6})
	if stale.Code != 409 {
		t.Fatalf("stale update: %d", stale.Code)
	}
	missing := f.request(&f.Admin, "PATCH", "/api/admin/settings", map[string]any{"login_absolute_hours": 6})
	if missing.Code != 400 {
		t.Fatalf("missing revision: %d", missing.Code)
	}
	rr := httptest.NewRecorder()
	f.H.setCookie(rr, "synthetic-token")
	if c := rr.Result().Cookies()[0]; c.MaxAge != 12*3600 {
		t.Fatalf("cookie lifetime: %d", c.MaxAge)
	}
	health := f.request(&f.Admin, "GET", "/api/admin/health", nil)
	var result struct {
		Recording bool `json:"recording_enabled"`
	}
	if e := json.Unmarshal(health.Body.Bytes(), &result); e != nil || !result.Recording {
		t.Fatalf("health must reflect policy: %s", health.Body.String())
	}
}

func TestRetentionRemovesExpiredShares(t *testing.T) {
	f := newFixture(t)
	id := f.connect(&f.Alice).ID
	s := control.Share{ID: "expired", Owner: f.Alice.User.ID, SessionID: id, LoginHash: control.TokenHash(f.Alice.Token), Expires: time.Now().Add(-time.Minute).Unix(), Recipients: []string{f.Bob.User.ID}}
	if e := f.Store.SaveShare(f.Alice.User.ID, s); e != nil {
		t.Fatal(e)
	}
	if e := f.H.retain(); e != nil {
		t.Fatal(e)
	}
	if _, e := f.Store.Share(s.ID); e == nil {
		t.Fatal("expired share still persisted")
	}
}

func TestHTTPSLoginCookieTransport(t *testing.T) {
	f := newFixture(t)
	f.H.config.DevHTTP = false
	server := httptest.NewTLSServer(f.Routes)
	defer server.Close()
	f.H.config.PublicURL = server.URL
	client := server.Client() // Trust only the certificate generated by this test server.
	client.Jar, _ = cookiejar.New(nil)
	response, e := client.Get(server.URL + "/api/auth/csrf")
	if e != nil {
		t.Fatal(e)
	}
	var csrf struct {
		Token string `json:"csrf_token"`
	}
	e = json.NewDecoder(response.Body).Decode(&csrf)
	response.Body.Close()
	if e != nil {
		t.Fatal(e)
	}
	req, _ := http.NewRequest("POST", server.URL+"/api/auth/login", strings.NewReader(`{"login":"alice","password":"test-password-123"}`))
	req.Header.Set("Origin", server.URL)
	req.Header.Set("X-CSRF-Token", csrf.Token)
	response, e = client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("login: %d", response.StatusCode)
	}
	loginCookies := response.Cookies()
	secure := false
	for _, c := range loginCookies {
		if c.Name == "__Host-conduit_session" {
			secure = c.Secure && c.HttpOnly && c.Path == "/" && c.Domain == "" && c.SameSite == http.SameSiteLaxMode
		}
	}
	if !secure {
		t.Fatal("missing protected session cookie")
	}
	response, e = client.Get(server.URL + "/api/auth/me")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("authenticated me: %d", response.StatusCode)
	}
	// Go and browsers treat loopback as a secure context. Check transport
	// restrictions separately against a non-loopback origin.
	protectedJar, _ := cookiejar.New(nil)
	origin, _ := url.Parse("https://conduit.test")
	protectedJar.SetCookies(origin, loginCookies)
	insecure, _ := url.Parse("http://conduit.test")
	for _, c := range protectedJar.Cookies(insecure) {
		if c.Name == "__Host-conduit_session" {
			t.Fatal("session sent over HTTP")
		}
	}
}
