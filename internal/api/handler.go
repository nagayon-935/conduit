package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/config"
	"github.com/nagayon-935/conduit/internal/control"
	"github.com/nagayon-935/conduit/internal/session"
	"github.com/nagayon-935/conduit/internal/sshconn"
	"github.com/nagayon-935/conduit/internal/vault"
)

const (
	contentTypeJSON   = "application/json"
	wsReadBufferSize  = 4096
	wsWriteBufferSize = 4096
)

// Handler is the root HTTP handler for the Conduit API.
type Handler struct {
	config   *config.Config
	sessions *session.Manager
	vault    vault.VaultClient
	dialer   sshconn.SSHDialer
	upgrader websocket.Upgrader
	control  *control.Store
	access   sync.Mutex
	pending  map[string]context.CancelFunc
	tickets  map[string]wsTicket
	sockets  map[string]socketAccess
	limits   map[string]loginLimit
}

// NewHandler constructs a Handler wiring together all application dependencies.
func NewHandler(cfg *config.Config, sm *session.Manager, vc vault.VaultClient, d sshconn.SSHDialer, store *control.Store) *Handler {
	h := &Handler{
		config:   cfg,
		sessions: sm,
		vault:    vc,
		dialer:   d,
		control:  store,
		pending:  make(map[string]context.CancelFunc), tickets: make(map[string]wsTicket), sockets: make(map[string]socketAccess), limits: make(map[string]loginLimit),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  wsReadBufferSize,
			WriteBufferSize: wsWriteBufferSize,
		},
	}
	h.upgrader.CheckOrigin = h.originOK
	return h
}

// Routes registers all API routes and returns the root http.Handler.
func (h *Handler) Routes() http.Handler {
	return h.secureRoutes()
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

// loggingMiddleware logs each incoming HTTP request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
