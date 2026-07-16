// Package api_test — admin endpoint authentication tests.
// GET /api/sessions and DELETE /api/sessions/{token} are gated by
// ADMIN_API_TOKEN. When unset, they stay open (lab default, backward
// compatible with existing tests in handler_test.go).
package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nagayon-935/conduit/internal/api"
	"github.com/nagayon-935/conduit/internal/config"
	"github.com/nagayon-935/conduit/internal/connlog"
	"github.com/nagayon-935/conduit/internal/session"
)

func newTestConfigWithAdminToken(token string) *config.Config {
	cfg := newTestConfig()
	cfg.AdminAPIToken = config.Secret(token)
	return cfg
}

func newTestHandlerWithConfig(cfg *config.Config, v *mockVaultClient, d *mockSSHDialer) http.Handler {
	sm := session.NewManager(cfg)
	h := api.NewHandler(cfg, sm, v, d, connlog.NewMemoryStore(100))
	return h.Routes()
}

// TestAdminEndpoints_NoTokenConfigured_RemainsOpen verifies backward
// compatibility: when ADMIN_API_TOKEN is unset (zero value), admin endpoints
// are not gated.
func TestAdminEndpoints_NoTokenConfigured_RemainsOpen(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(mockVaultOK(), mockDialerOK())

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (admin endpoints stay open when ADMIN_API_TOKEN unset)", w.Code)
	}
}

// TestAdminEndpoints_MissingAuthHeader_Returns401 verifies a request without
// an Authorization header is rejected once ADMIN_API_TOKEN is configured.
func TestAdminEndpoints_MissingAuthHeader_Returns401(t *testing.T) {
	t.Parallel()
	cfg := newTestConfigWithAdminToken("s3cr3t")
	handler := newTestHandlerWithConfig(cfg, mockVaultOK(), mockDialerOK())

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

// TestAdminEndpoints_WrongToken_Returns401 verifies an incorrect bearer token
// is rejected.
func TestAdminEndpoints_WrongToken_Returns401(t *testing.T) {
	t.Parallel()
	cfg := newTestConfigWithAdminToken("s3cr3t")
	handler := newTestHandlerWithConfig(cfg, mockVaultOK(), mockDialerOK())

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

// TestAdminEndpoints_CorrectToken_Returns200 verifies the correct bearer
// token is accepted.
func TestAdminEndpoints_CorrectToken_Returns200(t *testing.T) {
	t.Parallel()
	cfg := newTestConfigWithAdminToken("s3cr3t")
	handler := newTestHandlerWithConfig(cfg, mockVaultOK(), mockDialerOK())

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// TestAdminEndpoints_KillSession_RequiresToken verifies DELETE
// /api/sessions/{token} is also gated: rejected without auth, accepted with it.
func TestAdminEndpoints_KillSession_RequiresToken(t *testing.T) {
	t.Parallel()
	cfg := newTestConfigWithAdminToken("s3cr3t")
	handler := newTestHandlerWithConfig(cfg, mockVaultOK(), mockDialerOK())

	wConnect := postJSON(t, handler, "/api/connect", map[string]any{
		"host": "127.0.0.1", "port": 22, "user": "test",
	})
	body := decodeJSONBody(t, wConnect)
	token := body["session_token"].(string)

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/"+token, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status without token = %d, want 401", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodDelete, "/api/sessions/"+token, nil)
	req2.Header.Set("Authorization", "Bearer s3cr3t")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("status with token = %d, want 204", w2.Code)
	}
}
