package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/config"
	"github.com/nagayon-935/conduit/internal/connlog"
	"github.com/nagayon-935/conduit/internal/session"
	"github.com/nagayon-935/conduit/internal/sshconn"
	"github.com/nagayon-935/conduit/internal/vault"
)

const (
	contentTypeJSON    = "application/json"
	wsReadBufferSize   = 4096
	wsWriteBufferSize  = 4096
)

// Handler is the root HTTP handler for the Conduit API.
type Handler struct {
	config   *config.Config
	sessions *session.Manager
	vault    vault.VaultClient
	dialer   sshconn.SSHDialer
	upgrader websocket.Upgrader
	logs     connlog.Store
}

// NewHandler constructs a Handler wiring together all application dependencies.
func NewHandler(cfg *config.Config, sm *session.Manager, vc vault.VaultClient, d sshconn.SSHDialer, ls connlog.Store) *Handler {
	allowed := cfg.AllowedOrigins
	return &Handler{
		config:   cfg,
		sessions: sm,
		vault:    vc,
		dialer:   d,
		logs:     ls,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true // same-origin requests have no Origin header
				}
				return slices.Contains(allowed, origin)
			},
			ReadBufferSize:  wsReadBufferSize,
			WriteBufferSize: wsWriteBufferSize,
		},
	}
}

// Routes registers all API routes and returns the root http.Handler.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/connect", h.handleConnect)
	mux.HandleFunc("GET /ws", h.handleTerminal)
	mux.HandleFunc("GET /healthz", h.handleHealth)
	mux.HandleFunc("GET /api/sessions", h.handleListSessions)
	mux.HandleFunc("DELETE /api/sessions/{token}", h.handleKillSession)
	mux.HandleFunc("POST /api/sessions/{token}/share", h.handleCreateShare)
	mux.HandleFunc("DELETE /api/sessions/{token}/share/{shareToken}", h.handleRevokeShare)
	mux.HandleFunc("GET /api/logs", h.handleListLogs)
	mux.HandleFunc("GET /api/recordings/{id}", h.handleGetRecording)

	logged := corsMiddleware(h.config.AllowedOrigins)(loggingMiddleware(mux))
	return logged
}

// handleHealth is a simple liveness probe.
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// apiError writes a structured JSON error response.
func apiError(w http.ResponseWriter, code int, message, errCode string) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(code)
	body, _ := json.Marshal(map[string]string{
		"error": message,
		"code":  errCode,
	})
	_, _ = w.Write(body)
}

// writeJSON marshals v and writes it as a JSON response.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON: encode failed", "error", err)
	}
}

// corsMiddleware validates the Origin header against the configured allowlist
// and sets CORS response headers accordingly.
func corsMiddleware(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && slices.Contains(allowed, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// loggingMiddleware logs each incoming HTTP request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
