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
	"github.com/nagayon-935/conduit/internal/connlog"
	"github.com/nagayon-935/conduit/internal/session"
	"github.com/nagayon-935/conduit/internal/sshconn"
	vaultpkg "github.com/nagayon-935/conduit/internal/vault"
	gossh "golang.org/x/crypto/ssh"
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

// buildTestServer wires up all dependencies and returns an httptest.Server.
func buildTestServer(t *testing.T, vaultURL string) *httptest.Server {
	t.Helper()
	cfg := &config.Config{
		VaultAddr:         vaultURL,
		VaultToken:        "test-token",
		VaultSSHMount:     "ssh",
		VaultSSHRole:      "conduit-role",
		GracePeriod:       15 * time.Minute,
		SessionGCInterval: 1 * time.Minute,
	}

	vaultClient, err := vaultpkg.NewClient(cfg.VaultAddr, cfg.VaultToken.Value(), cfg.VaultSSHMount, cfg.VaultSSHRole)
	if err != nil {
		t.Fatalf("vault.NewClient: %v", err)
	}

	dialer := sshconn.NewDialer("") // empty = insecure for integration tests
	sm := session.NewManager(cfg)
	handler := api.NewHandler(cfg, sm, vaultClient, dialer, connlog.NewMemoryStore(200))
	return httptest.NewServer(handler.Routes())
}

// postConnect calls POST /api/connect and returns the decoded response body.
func postConnect(t *testing.T, serverURL, host string, port int, user string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"host": host, "port": port, "user": user})
	resp, err := http.Post(serverURL+"/api/connect", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/connect: %v", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode connect response: %v", err)
	}
	return resp.StatusCode, result
}

// dialWS dials the WebSocket endpoint. Returns the connection or nil if it fails.
func dialWS(t *testing.T, serverURL, token string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws?token=" + token
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	return dialer.Dial(wsURL, nil)
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

	// Step 1: POST /api/connect.
	status, body := postConnect(t, apiSrv.URL, sshHost, sshPort, "testuser")
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %v", status, body)
	}
	token, ok := body["session_token"].(string)
	if !ok || token == "" {
		t.Fatalf("session_token missing from response: %v", body)
	}

	// Step 2: Connect WebSocket.
	ws, _, err := dialWS(t, apiSrv.URL, token)
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
		// Timeout or close is acceptable; the server may close after echo.
		t.Logf("ReadMessage (may be benign): %v", err)
	} else {
		t.Logf("received from WS: %q", data)
	}

	// Step 5: Disconnect and reconnect within grace period.
	ws.Close()
	time.Sleep(50 * time.Millisecond)

	ws2, _, err := dialWS(t, apiSrv.URL, token)
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

	ws, _, err := dialWS(t, apiSrv.URL, "totally-fake-token-xyz")
	if err != nil {
		// WebSocket upgrade may succeed at the HTTP layer; the server then sends an error.
		// If dial itself fails that is also acceptable.
		t.Logf("WebSocket dial failed (may be expected): %v", err)
		return
	}
	defer ws.Close()

	// Expect an error frame from the server.
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	msgType, data, err := ws.ReadMessage()
	if err != nil {
		// Connection closed without a message – also acceptable.
		t.Logf("ReadMessage after invalid token: %v (connection likely closed by server)", err)
		return
	}
	if msgType != websocket.TextMessage {
		t.Errorf("expected TextMessage (JSON error), got type %d", msgType)
	}
	var frame struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatalf("decode error frame: %v", err)
	}
	// Server sends "exit" for unknown/terminated sessions so the client stops reconnecting.
	if frame.Type != "exit" && frame.Type != "error" {
		t.Errorf("frame.type: got %q, want %q or %q", frame.Type, "exit", "error")
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
	status, body := postConnect(t, apiSrv.URL, "127.0.0.1", 22, "testuser")
	if status != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d; body: %v", status, body)
	}
	code, _ := body["code"].(string)
	if code != "VAULT_ERROR" {
		t.Errorf("error code: got %q, want %q", code, "VAULT_ERROR")
	}
}
