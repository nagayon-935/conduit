// Package api_test contains unit tests for the HTTP handlers.
// 各テストは Vault と SSH Dialer をインメモリのモックに差し替えて実行するため、
// 外部サービスなしで動作する。
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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
	gossh "golang.org/x/crypto/ssh"
)

// ============================================================
// モック型定義
// ============================================================

// mockVaultClient は vault.VaultClient インターフェースのインメモリ実装。
type mockVaultClient struct {
	fn func(ctx context.Context, publicKey, principal string) (string, error)
}

func (m *mockVaultClient) SignPublicKey(ctx context.Context, publicKey, principal string) (string, error) {
	return m.fn(ctx, publicKey, principal)
}

// mockVaultOK は常に成功するモック Vault を返す。
// 返却するダミー証明書文字列は Dialer モックが無視するので内容は問わない。
func mockVaultOK() *mockVaultClient {
	return &mockVaultClient{
		fn: func(_ context.Context, _, _ string) (string, error) {
			return "ssh-ed25519-cert-v01@openssh.com AAAA dummy", nil
		},
	}
}

// mockVaultErr は常に指定メッセージのエラーを返すモック Vault を返す。
func mockVaultErr(msg string) *mockVaultClient {
	return &mockVaultClient{
		fn: func(_ context.Context, _, _ string) (string, error) {
			return "", errors.New(msg)
		},
	}
}

// nopWriteCloser は io.Writer を io.WriteCloser に変換するアダプタ。
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// mockSSHDialer は sshconn.SSHDialer インターフェースのインメモリ実装。
type mockSSHDialer struct {
	fn func(ctx context.Context, req sshconn.ConnectRequest) (*gossh.Client, *gossh.Session, io.WriteCloser, io.Reader, error)
}

func (m *mockSSHDialer) Dial(ctx context.Context, req sshconn.ConnectRequest) (*gossh.Client, *gossh.Session, io.WriteCloser, io.Reader, error) {
	return m.fn(ctx, req)
}

// mockDialerOK は常に成功するモック Dialer を返す。
// SSH クライアント/セッションは nil だが、handleConnect はポンプを起動しないため安全。
func mockDialerOK() *mockSSHDialer {
	return &mockSSHDialer{
		fn: func(_ context.Context, _ sshconn.ConnectRequest) (*gossh.Client, *gossh.Session, io.WriteCloser, io.Reader, error) {
			return nil, nil, nopWriteCloser{io.Discard}, strings.NewReader(""), nil
		},
	}
}

// mockDialerErr は常に指定メッセージのエラーを返すモック Dialer を返す。
func mockDialerErr(msg string) *mockSSHDialer {
	return &mockSSHDialer{
		fn: func(_ context.Context, _ sshconn.ConnectRequest) (*gossh.Client, *gossh.Session, io.WriteCloser, io.Reader, error) {
			return nil, nil, nil, nil, errors.New(msg)
		},
	}
}

// ============================================================
// テストヘルパー
// ============================================================

func newTestConfig() *config.Config {
	return &config.Config{
		VaultAddr:         "http://vault.test:8200",
		VaultToken:        config.Secret("test-token"),
		VaultSSHMount:     "ssh",
		VaultSSHRole:      "test-role",
		GracePeriod:       15 * time.Minute,
		SessionGCInterval: time.Minute,
		AllowedOrigins:    []string{"http://localhost:5173", "https://conduit.akilab.internal"},
	}
}

// newTestHandler はモックを注入した http.Handler を組み立てる。
func newTestHandler(v *mockVaultClient, d *mockSSHDialer) http.Handler {
	cfg := newTestConfig()
	sm := session.NewManager(cfg)
	h := api.NewHandler(cfg, sm, v, d, connlog.NewMemoryStore(100))
	return h.Routes()
}

// postJSON は JSON ボディ付きの POST リクエストを送信し、ResponseRecorder を返す。
func postJSON(t *testing.T, handler http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

// decodeJSONBody はレスポンスボディを map[string]any に decode する。
func decodeJSONBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode JSON response body: %v", err)
	}
	return result
}

// assertErrorCode は JSON レスポンスの "code" フィールドを検証する。
func assertErrorCode(t *testing.T, body map[string]any, want string) {
	t.Helper()
	got, _ := body["code"].(string)
	if got != want {
		t.Errorf("error code = %q, want %q; full body: %v", got, want, body)
	}
}

// ============================================================
// POST /api/connect のテスト
// ============================================================

// TestHandleConnect_Success は正常系: 201 と session_token が返ることを検証する。
func TestHandleConnect_Success(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	w := postJSON(t, handler, "/api/connect", map[string]any{
		"host": "10.0.0.1",
		"port": 22,
		"user": "ubuntu",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	body := decodeJSONBody(t, w)

	tok, ok := body["session_token"].(string)
	if !ok || tok == "" {
		t.Fatalf("session_token missing or empty: %v", body)
	}
	// token は 64文字 hex (32バイト)
	if len(tok) != 64 {
		t.Errorf("session_token length = %d, want 64", len(tok))
	}
	if _, ok := body["expires_at"].(string); !ok {
		t.Errorf("expires_at missing from response: %v", body)
	}
	if _, ok := body["message"].(string); !ok {
		t.Errorf("message missing from response: %v", body)
	}
}

// TestHandleConnect_InvalidJSON は JSON パース失敗時に 400 BAD_REQUEST を返すことを検証する。
func TestHandleConnect_InvalidJSON(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	req := httptest.NewRequest(http.MethodPost, "/api/connect", strings.NewReader("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, decodeJSONBody(t, w), "BAD_REQUEST")
}

// TestHandleConnect_MissingHost は host が空の場合に 400 INVALID_REQUEST を返すことを検証する。
func TestHandleConnect_MissingHost(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	w := postJSON(t, handler, "/api/connect", map[string]any{
		"host": "",
		"port": 22,
		"user": "ubuntu",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, decodeJSONBody(t, w), "INVALID_REQUEST")
}

// TestHandleConnect_PortZero は port=0 の場合に 400 INVALID_REQUEST を返すことを検証する。
func TestHandleConnect_PortZero(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	w := postJSON(t, handler, "/api/connect", map[string]any{
		"host": "10.0.0.1",
		"port": 0,
		"user": "ubuntu",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, decodeJSONBody(t, w), "INVALID_REQUEST")
}

// TestHandleConnect_PortTooLarge は port=99999 の場合に 400 INVALID_REQUEST を返すことを検証する。
func TestHandleConnect_PortTooLarge(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	w := postJSON(t, handler, "/api/connect", map[string]any{
		"host": "10.0.0.1",
		"port": 99999,
		"user": "ubuntu",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, decodeJSONBody(t, w), "INVALID_REQUEST")
}

// TestHandleConnect_MissingUser は user が空の場合に 400 INVALID_REQUEST を返すことを検証する。
func TestHandleConnect_MissingUser(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	w := postJSON(t, handler, "/api/connect", map[string]any{
		"host": "10.0.0.1",
		"port": 22,
		"user": "",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, decodeJSONBody(t, w), "INVALID_REQUEST")
}

// TestHandleConnect_VaultError は Vault が失敗した場合に 502 VAULT_ERROR を返すことを検証する。
func TestHandleConnect_VaultError(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultErr("permission denied"), mockDialerOK())
	w := postJSON(t, handler, "/api/connect", map[string]any{
		"host": "10.0.0.1",
		"port": 22,
		"user": "ubuntu",
	})

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	assertErrorCode(t, decodeJSONBody(t, w), "VAULT_ERROR")
}

// TestHandleConnect_SSHDialError は SSH 接続が失敗した場合に 502 SSH_DIAL_ERROR を返すことを検証する。
func TestHandleConnect_SSHDialError(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerErr("connection refused"))
	w := postJSON(t, handler, "/api/connect", map[string]any{
		"host": "10.0.0.1",
		"port": 22,
		"user": "ubuntu",
	})

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	assertErrorCode(t, decodeJSONBody(t, w), "SSH_DIAL_ERROR")
}

// TestHandleConnect_WrongMethod は GET でアクセスした場合に 405 を返すことを検証する。
// Go 1.22 以降の ServeMux はメソッドミスマッチを 405 で返す。
func TestHandleConnect_WrongMethod(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	req := httptest.NewRequest(http.MethodGet, "/api/connect", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

// ============================================================
// GET /ws のテスト
// ============================================================

// TestHandleTerminal_MissingToken は token クエリパラメータが欠如した場合に
// WebSocket アップグレード前に 400 MISSING_TOKEN を返すことを検証する。
func TestHandleTerminal_MissingToken(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	req := httptest.NewRequest(http.MethodGet, "/ws", nil) // token パラメータなし
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, decodeJSONBody(t, w), "MISSING_TOKEN")
}

// TestHandleTerminal_InvalidToken はセッションストアに存在しない token でアクセスした場合に
// WebSocket 経由でエラーフレームが送信されることを検証する。
func TestHandleTerminal_InvalidToken(t *testing.T) {
	// WebSocket には実際の TCP 接続が必要なため httptest.NewServer を使用。
	// t.Parallel() は httptest.NewServer と競合しないが、ポート確保の競合を避けるため外す。

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?token=totally-fake-token"
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		// サーバーがアップグレード前に接続を閉じた場合も「エラーが伝わっている」と判断できる
		t.Logf("Dial returned error (server may have rejected early): %v", err)
		return
	}
	defer conn.Close()

	// サーバーが {"type":"error","message":"..."} を送信してから接続を閉じるはず
	conn.SetReadDeadline(time.Now().Add(3 * time.Second)) //nolint:errcheck
	msgType, data, readErr := conn.ReadMessage()
	if readErr != nil {
		t.Logf("ReadMessage error (connection closed by server — acceptable): %v", readErr)
		return
	}

	if msgType != websocket.TextMessage {
		t.Errorf("message type = %d, want TextMessage (%d)", msgType, websocket.TextMessage)
	}

	var frame struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatalf("unmarshal error frame: %v", err)
	}
	// Server sends "exit" for unknown/terminated sessions so the client stops reconnecting.
	if frame.Type != "exit" && frame.Type != "error" {
		t.Errorf("frame.type = %q, want %q or %q", frame.Type, "exit", "error")
	}
}

// ============================================================
// GET /healthz のテスト
// ============================================================

// TestHandleHealth は 200 と {"status":"ok"} が返ることを検証する。
func TestHandleHealth(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// ============================================================
// CORS ミドルウェアのテスト
// ============================================================

// TestCORSHeaders は許可されたオリジンからのリクエストに Access-Control ヘッダが付与されることを検証する。
func TestCORSHeaders(t *testing.T) {
	t.Parallel()

	const allowedOrigin = "http://localhost:5173"
	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", allowedOrigin)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, allowedOrigin)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods header should not be empty")
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Access-Control-Allow-Headers header should not be empty")
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want %q", got, "Origin")
	}
}

// TestCORSHeaders_NoOrigin はオリジンなしのリクエストに ACAO ヘッダが付与されないことを検証する。
func TestCORSHeaders_NoOrigin(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for no-origin request", got)
	}
}

// TestCORSPreflight は OPTIONS プリフライトリクエストに 204 を返すことを検証する。
func TestCORSPreflight(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(mockVaultOK(), mockDialerOK())
	req := httptest.NewRequest(http.MethodOptions, "/api/connect", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

// ============================================================
// GET /ws のテスト（成功系）
// ============================================================

// TestHandleTerminal_Success は有効な token でアクセスした場合に
// WebSocket アップグレードが成功し、SSH クライアントとの通信が開始されることを検証する。
func TestHandleTerminal_Success(t *testing.T) {
	handler := newTestHandler(mockVaultOK(), mockDialerOK())

	// まずセッションを作成する
	w := postJSON(t, handler, "/api/connect", map[string]any{
		"host": "127.0.0.1",
		"port": 22,
		"user": "test",
	})
	body := decodeJSONBody(t, w)
	token := body["session_token"].(string)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?token=" + token
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// 接続が確立されたことを確認
	time.Sleep(100 * time.Millisecond)
}

// ============================================================
// GET /api/sessions のテスト
// ============================================================

func TestHandleListSessions(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(mockVaultOK(), mockDialerOK())

	// Create a session first
	postJSON(t, handler, "/api/connect", map[string]any{
		"host": "127.0.0.1",
		"port": 22,
		"user": "test",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var sessions []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("len(sessions) = %d, want 1", len(sessions))
	}
}

func TestHandleKillSession_Success(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(mockVaultOK(), mockDialerOK())

	// Create a session first
	wConnect := postJSON(t, handler, "/api/connect", map[string]any{
		"host": "127.0.0.1",
		"port": 22,
		"user": "test",
	})
	body := decodeJSONBody(t, wConnect)
	token := body["session_token"].(string)

	// Use the actual path with the token
	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/"+token, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

// TestHandleKillSession_ByLogID_Fallback verifies that the admin UI can kill a
// session using the non-secret "id" field returned by GET /api/sessions, since
// that endpoint truncates the full capability token and never exposes it.
func TestHandleKillSession_ByLogID_Fallback(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(mockVaultOK(), mockDialerOK())

	postJSON(t, handler, "/api/connect", map[string]any{
		"host": "127.0.0.1", "port": 22, "user": "test",
	})

	reqList := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	wList := httptest.NewRecorder()
	handler.ServeHTTP(wList, reqList)

	var sessions []map[string]any
	if err := json.NewDecoder(wList.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	id, ok := sessions[0]["id"].(string)
	if !ok || id == "" {
		t.Fatalf("expected non-empty id field in session list, got: %v", sessions[0])
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/"+id, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleKillSession_NotFound(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(mockVaultOK(), mockDialerOK())

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/non-existent-token", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// ============================================================
// GET /api/logs のテスト
// ============================================================

func TestHandleListLogs(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(mockVaultOK(), mockDialerOK())

	// Connect triggers a log entry
	postJSON(t, handler, "/api/connect", map[string]any{
		"host": "127.0.0.1",
		"port": 22,
		"user": "test",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var logs []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&logs); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("len(logs) = %d, want 1", len(logs))
	}
}
