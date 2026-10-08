package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/api"
	"github.com/nagayon-935/conduit/internal/connlog"
	"github.com/nagayon-935/conduit/internal/session"
)

func TestWorkspaceOwnerAndAdminBoundaries(t *testing.T) {
	cfg := newTestConfigWithAdminToken("admin-only")
	manager := session.NewManager(cfg)
	owner := session.NewSession("owner-secret", "host", 22, "user", nil, nil, nil, nil, time.Minute)
	owner.LogID = "public-id"
	if err := manager.Create(owner); err != nil {
		t.Fatal(err)
	}
	share, _, err := manager.Share(owner.Token)
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewHandler(cfg, manager, mockVaultOK(), mockDialerOK(), connlog.NewMemoryStore(10)).Routes()
	request := func(method, path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("capability response is cacheable")
		}
		return w
	}
	for _, token := range []string{"", share, "public-id", "admin-only"} {
		if w := request("DELETE", "/api/session", token); w.Code == 204 {
			t.Fatalf("non-owner %q ended session", token)
		}
	}
	for _, path := range []string{"/api/sessions", "/api/logs", "/api/recordings/missing"} {
		if w := request("GET", path, owner.Token); w.Code != 401 {
			t.Fatalf("%s: owner accessed admin endpoint: %d", path, w.Code)
		}
	}
	if w := request("GET", "/api/shared-session/"+share, ""); w.Code != 200 {
		t.Fatalf("shared status: %d", w.Code)
	}
	w := request("GET", "/api/session", owner.Token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "public-id") {
		t.Fatalf("owner state: %d %s", w.Code, w.Body.String())
	}
	if w := request("GET", "/api/session/shares", owner.Token); w.Code != 200 || !strings.Contains(w.Body.String(), share) {
		t.Fatal("share list missing")
	}
	if w := request("GET", "/api/sessions", "admin-only"); w.Code != 200 || strings.Contains(w.Body.String(), owner.Token) {
		t.Fatalf("admin list leaks capability: %s", w.Body.String())
	}
	if w := request("DELETE", "/api/session", owner.Token); w.Code != 204 {
		t.Fatalf("end: %d", w.Code)
	}
	if w := request("GET", "/api/session", owner.Token); w.Code != 410 {
		t.Fatalf("ended: %d", w.Code)
	}
	if w := request("GET", "/api/shared-session/"+share, ""); w.Code != 410 {
		t.Fatalf("ended shared status: %d", w.Code)
	}
	if owner.Info().EndReason == "" {
		t.Fatal("end reason missing")
	}
}

func TestRevokeShareDisconnectsCurrentViewerAndRejectsOtherOwner(t *testing.T) {
	cfg := newTestConfig()
	manager := session.NewManager(cfg)
	owner := session.NewSession("owner", "host", 22, "user", nil, nil, nil, nil, time.Minute)
	owner.StartOnce(func() {}) // no SSH process required for the sharing lifecycle
	_ = manager.Create(owner)
	other := session.NewSession("other", "host", 22, "user", nil, nil, nil, nil, time.Minute)
	_ = manager.Create(other)
	share, _, err := manager.Share(owner.Token)
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewHandler(cfg, manager, mockVaultOK(), mockDialerOK(), connlog.NewMemoryStore(10)).Routes()
	server := httptest.NewServer(handler)
	defer server.Close()
	defer owner.Close()
	viewer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws?share="+share, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetReadDeadline(time.Now().Add(time.Second))
	_, frame, err := viewer.ReadMessage()
	if err != nil || !bytes.Contains(frame, []byte(`"type":"session"`)) {
		t.Fatalf("initial state: %s %v", frame, err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/sessions/other/share/"+share, nil))
	if w.Code != 404 {
		t.Fatalf("other owner revoked share: %d", w.Code)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/sessions/owner/share/"+share, nil))
	if w.Code != 204 {
		t.Fatalf("revoke: %d", w.Code)
	}
	_, frame, err = viewer.ReadMessage()
	if err != nil || !bytes.Contains(frame, []byte(`"type":"exit"`)) {
		t.Fatalf("viewer exit: %s %v", frame, err)
	}
	if _, _, err = viewer.ReadMessage(); err == nil {
		t.Fatal("viewer remains connected")
	}
	if _, ok := manager.ResolveShare(share); ok {
		t.Fatal("revoked share resolves")
	}
}

func TestConnectedShareExpiresAtDeadline(t *testing.T) {
	cfg := newTestConfig()
	manager := session.NewManager(cfg)
	owner := session.NewSession("owner", "host", 22, "user", nil, nil, nil, nil, time.Minute)
	owner.StartOnce(func() {})
	_ = manager.Create(owner)
	defer owner.Close()
	share, _, err := manager.ShareFor(owner.Token, 150*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewHandler(cfg, manager, mockVaultOK(), mockDialerOK(), connlog.NewMemoryStore(10)).Routes())
	defer server.Close()
	viewer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws?share="+share, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, frame, err := viewer.ReadMessage()
		if err != nil {
			t.Fatalf("missing expiry reason: %v", err)
		}
		var message struct{ Type string }
		_ = json.Unmarshal(frame, &message)
		if message.Type == "exit" {
			break
		}
	}
	if _, _, err := viewer.ReadMessage(); err == nil {
		t.Fatal("expired viewer stays connected")
	}
}

func TestSSHFinalOutputPrecedesExit(t *testing.T) {
	cfg := newTestConfig()
	manager := session.NewManager(cfg)
	reader, writer := io.Pipe()
	owner := session.NewSession("owner", "host", 22, "user", nil, nil, nopWriteCloser{io.Discard}, reader, time.Minute)
	_ = manager.Create(owner)
	defer owner.Close()
	defer reader.Close()
	defer writer.Close()
	server := httptest.NewServer(api.NewHandler(cfg, manager, mockVaultOK(), mockDialerOK(), connlog.NewMemoryStore(10)).Routes())
	defer server.Close()
	terminal, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws?token=owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	terminal.SetReadDeadline(time.Now().Add(2 * time.Second))
	go func() { _, _ = writer.Write([]byte("final command output")); _ = writer.Close() }()
	output := ""
	for {
		typ, data, err := terminal.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if typ == websocket.BinaryMessage {
			output += string(data)
			continue
		}
		var message struct{ Type string }
		_ = json.Unmarshal(data, &message)
		if message.Type == "exit" {
			break
		}
	}
	if output != "final command output" {
		t.Fatalf("output lost before exit: %q", output)
	}
}
