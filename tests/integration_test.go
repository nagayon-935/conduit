// Package integration_test contains end-to-end tests that wire together
// the real HTTP API, a mock Vault HTTP server, and a real in-process SSH server.
package integration_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/api"
	"github.com/nagayon-935/conduit/internal/config"
	"github.com/nagayon-935/conduit/internal/control"
	"github.com/nagayon-935/conduit/internal/session"
	"github.com/nagayon-935/conduit/internal/sshconn"
	vaultpkg "github.com/nagayon-935/conduit/internal/vault"
	gossh "golang.org/x/crypto/ssh"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
)

// ---------- test helpers ----------

func genEd25519(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genEd25519: %v", err)
	}
	return priv
}

func signSSHCert(pub gossh.PublicKey, caSigner gossh.Signer, principals []string) (string, error) {
	cert := &gossh.Certificate{
		Key:             pub,
		CertType:        gossh.UserCert,
		ValidPrincipals: principals,
		ValidAfter:      0,
		ValidBefore:     gossh.CertTimeInfinity,
	}
	if err := cert.SignCert(rand.Reader, caSigner); err != nil {
		return "", fmt.Errorf("signSSHCert: %w", err)
	}
	return string(gossh.MarshalAuthorizedKey(cert)), nil
}

// mockVaultServer returns an httptest.Server that parses the public_key from the
// sign request and returns a certificate signed by caPriv.
func mockVaultServer(t *testing.T, caPriv gossh.Signer, principals []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/sign/") {
			http.NotFound(w, r)
			return
		}

		var body struct {
			PublicKey       string `json:"public_key"`
			ValidPrincipals string `json:"valid_principals"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(body.PublicKey))
		if err != nil {
			http.Error(w, "cannot parse public key: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Use the requested principals if provided, otherwise use the preset ones.
		usePrincipals := principals
		if body.ValidPrincipals != "" {
			usePrincipals = strings.Split(body.ValidPrincipals, ",")
		}

		certStr, err := signSSHCert(pub, caPriv, usePrincipals)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := map[string]any{
			"data": map[string]string{
				"signed_key": certStr,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// mockVaultServerError returns an httptest.Server that always returns HTTP 500.
func mockVaultServerError(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"errors": []string{"internal server error"}}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// startE2ESSHServer starts an in-process SSH server that accepts user certificates
// signed by caPubKey and echoes session data.
func startE2ESSHServer(t *testing.T, caPubKey gossh.PublicKey) (host string, port int, cleanup func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startE2ESSHServer listen: %v", err)
	}
	port = listener.Addr().(*net.TCPAddr).Port
	host = "127.0.0.1"

	cfg := &gossh.ServerConfig{
		PublicKeyCallback: func(conn gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			cert, ok := key.(*gossh.Certificate)
			if !ok {
				return nil, fmt.Errorf("not a certificate")
			}
			checker := &gossh.CertChecker{
				IsUserAuthority: func(auth gossh.PublicKey) bool {
					return bytes.Equal(auth.Marshal(), caPubKey.Marshal())
				},
			}
			return checker.Authenticate(conn, cert)
		},
	}
	hostKey := genEd25519(t)
	hostSigner, err := gossh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatalf("startE2ESSHServer host signer: %v", err)
	}
	cfg.AddHostKey(hostSigner)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleE2ESSHConn(conn, cfg)
		}
	}()

	return host, port, func() { listener.Close() }
}

func handleE2ESSHConn(conn net.Conn, cfg *gossh.ServerConfig) {
	sshConn, chans, reqs, err := gossh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go gossh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(gossh.UnknownChannelType, "unknown")
			continue
		}
		ch, requests, err := newChan.Accept()
		if err != nil {
			return
		}
		go func(ch gossh.Channel, reqs <-chan *gossh.Request) {
			defer ch.Close()
			for req := range reqs {
				switch req.Type {
				case "pty-req":
					_ = req.Reply(true, nil)
				case "shell":
					_ = req.Reply(true, nil)
					// Echo back whatever is written.
					go io.Copy(ch, ch)
				case "window-change":
					_ = req.Reply(true, nil)
				default:
					_ = req.Reply(false, nil)
				}
			}
		}(ch, requests)
	}
}

type testServer struct {
	*httptest.Server
	Client *http.Client
	Store  *control.Store
}

func buildTestServer(t *testing.T, vaultURL string) *testServer {
	t.Helper()
	cfg := &config.Config{DevHTTP: true, AllowedCIDRs: []string{"127.0.0.0/8"}, VaultAddr: vaultURL, VaultToken: "test-token", VaultSSHMount: "ssh", VaultSSHRole: "conduit-role", GracePeriod: 15 * time.Minute, SessionGCInterval: time.Minute}
	vc, err := vaultpkg.NewClient(cfg.VaultAddr, cfg.VaultToken.Value(), cfg.VaultSSHMount, cfg.VaultSSHRole)
	if err != nil {
		t.Fatal(err)
	}
	store, err := control.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Bootstrap("testuser", "temporary-password"); err != nil {
		t.Fatal(err)
	}
	h := api.NewHandler(cfg, session.NewManager(cfg), vc, sshconn.NewDialer(""), store)
	srv := httptest.NewServer(h.Routes())
	jar, _ := cookiejar.New(nil)
	out := &testServer{srv, &http.Client{Jar: jar}, store}
	t.Cleanup(func() { h.Close(); store.Close() })
	status, _ := requestJSON(t, out, "POST", "/api/auth/login", map[string]string{"login": "testuser", "password": "temporary-password"})
	if status != 200 {
		t.Fatal("login failed", status)
	}
	status, _ = requestJSON(t, out, "POST", "/api/auth/password", map[string]string{"current_password": "temporary-password", "password": "normal-password-123"})
	if status != 200 {
		t.Fatal("password change failed", status)
	}
	return out
}
func requestJSON(t *testing.T, s *testServer, method, path string, body any) (int, map[string]any) {
	t.Helper()
	csrfResp, e := s.Client.Get(s.URL + "/api/auth/csrf")
	if e != nil {
		t.Fatal(e)
	}
	var csrf struct {
		Token string `json:"csrf_token"`
	}
	if e = json.NewDecoder(csrfResp.Body).Decode(&csrf); e != nil {
		t.Fatal(e)
	}
	csrfResp.Body.Close()
	data, _ := json.Marshal(body)
	r, _ := http.NewRequest(method, s.URL+path, bytes.NewReader(data))
	r.Header.Set("Origin", s.URL)
	r.Header.Set("X-CSRF-Token", csrf.Token)
	r.Header.Set("Content-Type", "application/json")
	resp, e := s.Client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	if resp.StatusCode != 204 {
		if e = json.NewDecoder(resp.Body).Decode(&out); e != nil {
			t.Fatal(e)
		}
	}
	return resp.StatusCode, out
}
func postConnect(t *testing.T, s *testServer, host string, port int, user string) (int, map[string]any) {
	t.Helper()
	status, target := requestJSON(t, s, "POST", "/api/admin/targets", map[string]any{"name": "Integration SSH", "host": host, "port": port, "enabled": true})
	if status != 200 {
		t.Fatal(status, target)
	}
	status, acct := requestJSON(t, s, "POST", "/api/admin/targets/"+target["id"].(string)+"/accounts", map[string]any{"ssh_username": user, "auth_type": "vault", "enabled": true})
	if status != 200 {
		t.Fatal(status, acct)
	}
	uid := mustUser(t, s.Store)
	status, grant := requestJSON(t, s, "PUT", "/api/admin/users/"+uid+"/access-grants/"+acct["id"].(string), map[string]any{"can_connect": true})
	if status != 204 {
		t.Fatal(status, grant)
	}
	return requestJSON(t, s, "POST", "/api/app/sessions", map[string]any{"target_account_id": acct["id"]})
}
func mustUser(t *testing.T, s *control.Store) string {
	t.Helper()
	users, e := s.Users()
	if e != nil || len(users) != 1 {
		t.Fatal(users, e)
	}
	return users[0].ID
}
func dialWS(t *testing.T, s *testServer, id string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	status, ticket := requestJSON(t, s, "POST", "/api/app/sessions/"+id+"/ws-ticket", map[string]any{})
	if status != 201 {
		return nil, &http.Response{StatusCode: status}, fmt.Errorf("ticket denied: %v", ticket)
	}
	u, _ := url.Parse(s.URL + "/ws")
	headers := http.Header{}
	headers.Set("Origin", s.URL)
	for _, c := range s.Client.Jar.Cookies(u) {
		headers.Add("Cookie", c.Name+"="+c.Value)
	}
	return (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).Dial("ws"+strings.TrimPrefix(s.URL, "http")+"/ws?ticket="+ticket["ticket"].(string), headers)
}

// ---------- tests ----------

func TestEndToEnd_ConnectAndTerminal(t *testing.T) {
	// Generate CA key – both the mock Vault and the SSH server share this.
	caPriv := genEd25519(t)
	caSigner, err := gossh.NewSignerFromKey(caPriv)
	if err != nil {
		t.Fatalf("ca signer: %v", err)
	}
	caPub := caSigner.PublicKey()

	// Start mock Vault.
	vaultSrv := mockVaultServer(t, caSigner, []string{"testuser"})
	defer vaultSrv.Close()

	// Start real in-process SSH server.
	sshHost, sshPort, sshCleanup := startE2ESSHServer(t, caPub)
	defer sshCleanup()

	// Start the API server.
	apiSrv := buildTestServer(t, vaultSrv.URL)
	defer apiSrv.Close()

	// Step 1: Authenticate, register a target/account, grant access, and connect.
	status, body := postConnect(t, apiSrv, sshHost, sshPort, "testuser")
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %v", status, body)
	}
	token, ok := body["id"].(string)
	if !ok || token == "" {
		t.Fatalf("session ID missing from response: %v", body)
	}

	// Step 2: Connect WebSocket.
	ws, _, err := dialWS(t, apiSrv, token)
	if err != nil {
		t.Fatalf("WebSocket dial: %v", err)
	}
	defer ws.Close()

	// Step 3: Send a command via WebSocket binary message.
	cmd := []byte("echo hello\n")
	if err := ws.WriteMessage(websocket.BinaryMessage, cmd); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	// Step 4: Read a response back (the echo server echoes what we send).
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("terminal echo failed: %v", err)
	}
	if !bytes.Equal(data, cmd) {
		t.Fatalf("terminal output %q, want %q", data, cmd)
	}

	// Step 5: Disconnect and reconnect within grace period.
	ws.Close()
	time.Sleep(50 * time.Millisecond)

	ws2, _, err := dialWS(t, apiSrv, token)
	if err != nil {
		t.Fatalf("WebSocket reconnect: %v", err)
	}
	defer ws2.Close()
	t.Logf("reconnect succeeded")
}

func TestEndToEnd_InvalidToken(t *testing.T) {
	// Build a minimal API server (Vault URL doesn't matter for this test).
	caPriv := genEd25519(t)
	caSigner, err := gossh.NewSignerFromKey(caPriv)
	if err != nil {
		t.Fatalf("ca signer: %v", err)
	}
	vaultSrv := mockVaultServer(t, caSigner, []string{"testuser"})
	defer vaultSrv.Close()

	apiSrv := buildTestServer(t, vaultSrv.URL)
	defer apiSrv.Close()

	ws, resp, err := dialWS(t, apiSrv, "totally-fake-token-xyz")
	if err == nil {
		ws.Close()
		t.Fatal("unknown session issued ticket")
	}
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}

}

func TestEndToEnd_VaultFailure(t *testing.T) {
	// Mock Vault returns 500.
	vaultSrv := mockVaultServerError(t)
	defer vaultSrv.Close()

	// We still need an SSH server entry in the connect body, but it won't be reached.
	apiSrv := buildTestServer(t, vaultSrv.URL)
	defer apiSrv.Close()

	// Use a dummy SSH host – the request should fail at the Vault signing step.
	status, body := postConnect(t, apiSrv, "127.0.0.1", 22, "testuser")
	if status != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d; body: %v", status, body)
	}
	code, _ := body["code"].(string)
	if code != "AUTH_SETUP_ERROR" {
		t.Errorf("error code: got %q, want %q", code, "AUTH_SETUP_ERROR")
	}
}
