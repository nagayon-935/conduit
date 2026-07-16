package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
)

// requireAdmin gates admin endpoints behind ADMIN_API_TOKEN.
// Empty configuration keeps the endpoints open (lab default) but logs a
// warning so the gap is visible; set ADMIN_API_TOKEN in production.
func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := h.config.AdminAPIToken.Value()
		if want == "" {
			slog.Warn("admin endpoint accessed without ADMIN_API_TOKEN configured",
				"method", r.Method, "path", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			apiError(w, http.StatusUnauthorized, "admin token required", "UNAUTHORIZED")
			return
		}
		next.ServeHTTP(w, r)
	})
}
