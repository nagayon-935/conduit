package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/nagayon-935/conduit/internal/control"
)

type identity struct {
	User  control.User
	Login control.Login
}
type identityKey struct{}
type loginLimit struct {
	Count int
	Until time.Time
}

func who(r *http.Request) identity { return r.Context().Value(identityKey{}).(identity) }
func (h *Handler) cookieName() string {
	if h.config.DevHTTP {
		return "conduit_session"
	}
	return "__Host-conduit_session"
}
func (h *Handler) setCookie(w http.ResponseWriter, t string) {
	age := h.policy().LoginHours * 3600
	if t == "" {
		age = -1
	}
	http.SetCookie(w, &http.Cookie{Name: h.cookieName(), Value: t, Path: "/", Secure: !h.config.DevHTTP, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func (h *Handler) originOK(r *http.Request) bool {
	u, e := url.Parse(r.Header.Get("Origin"))
	if e != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	expected := h.config.PublicURL
	if expected == "" {
		scheme := "https"
		if h.config.DevHTTP {
			scheme = "http"
		}
		expected = scheme + "://" + r.Host
	}
	return u.Scheme+"://"+u.Host == expected
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		apiError(w, 400, "入力を確認してください", "BAD_REQUEST")
		return false
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		apiError(w, 400, "JSON は一つだけ指定してください", "BAD_REQUEST")
		return false
	}
	return true
}
func unavailable(w http.ResponseWriter, err error) {
	apiError(w, 503, "データベース操作を完了できませんでした", "STORAGE_ERROR")
}
func authFailure(w http.ResponseWriter, e error) {
	if errors.Is(e, sql.ErrNoRows) || errors.Is(e, control.ErrLoginExpired) {
		apiError(w, 401, "ログインし直してください", "UNAUTHENTICATED")
	} else {
		unavailable(w, e)
	}
}
func missing(w http.ResponseWriter)            { apiError(w, 404, "対象が見つかりません", "NOT_FOUND") }
func invalid(w http.ResponseWriter, err error) { apiError(w, 400, err.Error(), "INVALID_REQUEST") }
func (h *Handler) require(admin bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if h.control == nil {
			unavailable(w, errors.New("identity database required"))
			return
		}
		cookie, e := r.Cookie(h.cookieName())
		if e != nil {
			apiError(w, 401, "ログインしてください", "UNAUTHENTICATED")
			return
		}
		l, u, e := h.control.Login(control.TokenHash(cookie.Value))
		if e != nil {
			authFailure(w, e)
			return
		}
		if u.MustChange && r.URL.Path != "/api/auth/me" && r.URL.Path != "/api/auth/password" && r.URL.Path != "/api/auth/logout" {
			apiError(w, 403, "先にパスワードを変更してください", "PASSWORD_CHANGE_REQUIRED")
			return
		}
		if admin && u.Role != "admin" {
			apiError(w, 403, "管理者の権限が必要です", "FORBIDDEN")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if !h.originOK(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(l.CSRF)) != 1 {
				apiError(w, 403, "リクエストの確認に失敗しました", "CSRF_FAILED")
				return
			}
			if e = h.control.Touch(l.Hash); e != nil {
				unavailable(w, e)
				return
			}
		}
		next(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity{u, l})))
	}
}
func (h *Handler) csrf(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.control == nil {
		unavailable(w, errors.New("no database"))
		return
	}
	if c, e := r.Cookie(h.cookieName()); e == nil {
		if l, _, e := h.control.Login(control.TokenHash(c.Value)); e == nil {
			writeJSON(w, 200, map[string]string{"csrf_token": l.CSRF})
			return
		} else if !errors.Is(e, sql.ErrNoRows) && !errors.Is(e, control.ErrLoginExpired) {
			unavailable(w, e)
			return
		}
	}
	t := control.Random()
	http.SetCookie(w, &http.Cookie{Name: "conduit_login_csrf", Value: t, Path: "/", Secure: !h.config.DevHTTP, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 600})
	writeJSON(w, 200, map[string]string{"csrf_token": t})
}
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.control == nil {
		unavailable(w, errors.New("no database"))
		return
	}
	c, e := r.Cookie("conduit_login_csrf")
	if e != nil || !h.originOK(r) || subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
		apiError(w, 403, "ログイン画面を更新してください", "CSRF_FAILED")
		return
	}
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	ip := h.clientIP(r)
	keys := []string{"ip:" + ip, "login:" + control.Normalize(req.Login)}
	h.access.Lock()
	now := time.Now()
	for k, v := range h.limits {
		if now.After(v.Until) {
			delete(h.limits, k)
		}
	}
	for _, k := range keys {
		v := h.limits[k]
		max := 10
		if strings.HasPrefix(k, "ip:") {
			max = 60
		}
		if v.Count >= max {
			h.access.Unlock()
			apiError(w, 429, "しばらく待ってからお試しください", "RATE_LIMITED")
			return
		}
	}
	if len(h.limits) > 10000 {
		h.access.Unlock()
		apiError(w, 429, "しばらく待ってからお試しください", "RATE_LIMITED")
		return
	}
	for _, k := range keys {
		v := h.limits[k]
		if v.Count == 0 {
			v.Until = now.Add(15 * time.Minute)
		}
		v.Count++
		h.limits[k] = v
	}
	h.access.Unlock()
	u, e := h.control.Authenticate(req.Login, req.Password)
	if e != nil {
		if errors.Is(e, control.ErrCredentials) {
			apiError(w, 401, "ログイン名またはパスワードが違います", "INVALID_LOGIN")
		} else {
			unavailable(w, e)
		}
		return
	}
	h.access.Lock()
	defer h.access.Unlock()
	current, e := h.control.User(u.ID)
	if e != nil || !current.Enabled || current.Version != u.Version {
		apiError(w, 401, "ログインし直してください", "INVALID_LOGIN")
		return
	}
	if old, e := r.Cookie(h.cookieName()); e == nil {
		hash := control.TokenHash(old.Value)
		if e = h.control.Revoke(hash); e != nil {
			unavailable(w, e)
			return
		}
		h.revokeLoginLocked(hash)
	}
	token, csrf, e := h.control.NewLogin(u)
	if e != nil {
		unavailable(w, e)
		return
	}
	h.setCookie(w, token)
	writeJSON(w, 200, map[string]any{"user": u, "csrf_token": csrf, "expires_at": time.Now().Add(time.Duration(h.policy().LoginHours) * time.Hour).Unix()})
}
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	i := who(r)
	writeJSON(w, 200, map[string]any{"user": i.User, "csrf_token": i.Login.CSRF, "expires_at": i.Login.Created + int64(h.policy().LoginHours)*3600, "idle_expires_at": i.Login.Active + int64(h.policy().LoginIdleMinutes)*60, "reconnect_grace_minutes": h.policy().GraceMinutes})
}
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	i := who(r)
	var req struct {
		Terminate bool `json:"terminate_sessions"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	if e := h.control.Revoke(i.Login.Hash); e != nil {
		unavailable(w, e)
		return
	}
	h.revokeLoginLocked(i.Login.Hash)
	if req.Terminate {
		h.terminateOwner(i.User.ID, "owner_logout")
	}
	h.setCookie(w, "")
	w.WriteHeader(204)
}
func (h *Handler) password(w http.ResponseWriter, r *http.Request) {
	i := who(r)
	var req struct {
		Current  string `json:"current_password"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !control.Verify(req.Current, i.User.Hash) {
		apiError(w, 403, "現在のパスワードが違います", "INVALID_PASSWORD")
		return
	}
	hash, e := control.HashPassword(req.Password)
	if e != nil {
		invalid(w, e)
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	u, e := h.control.User(i.User.ID)
	if e != nil {
		unavailable(w, e)
		return
	}
	if u.Version != i.User.Version {
		apiError(w, 401, "ログインし直してください", "UNAUTHENTICATED")
		return
	}
	u.Hash = hash
	u.MustChange = false
	if e = h.control.SaveUser(u.ID, u); e != nil {
		unavailable(w, e)
		return
	}
	h.revokeUserLocked(u.ID, false)
	u, e = h.control.User(u.ID)
	if e != nil {
		unavailable(w, e)
		return
	}
	token, csrf, e := h.control.NewLogin(u)
	if e != nil {
		unavailable(w, e)
		return
	}
	h.setCookie(w, token)
	writeJSON(w, 200, map[string]any{"user": u, "csrf_token": csrf, "expires_at": time.Now().Add(time.Duration(h.policy().LoginHours) * time.Hour).Unix()})
}
func (h *Handler) secureRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handleHealth)
	mux.HandleFunc("GET /api/auth/csrf", h.csrf)
	mux.HandleFunc("POST /api/auth/login", h.login)
	for pattern, fn := range map[string]http.HandlerFunc{"GET /api/auth/me": h.me, "POST /api/auth/logout": h.logout, "POST /api/auth/password": h.password, "GET /api/app/targets": h.appTargets, "POST /api/app/sessions": h.connectRegistered, "GET /api/app/sessions": h.appSessions, "GET /api/app/sessions/{id}": h.appSession, "DELETE /api/app/sessions/{id}": h.endSession, "POST /api/app/sessions/{id}/ws-ticket": h.issueTicket, "GET /api/app/sessions/{id}/share-candidates": h.shareCandidates, "GET /api/app/sessions/{id}/shares": h.shares, "POST /api/app/sessions/{id}/shares": h.shares, "DELETE /api/app/sessions/{id}/shares/{share}": h.deleteShare, "GET /api/app/shared/{share}": h.shared, "POST /api/app/shared/{share}/ws-ticket": h.issueTicket, "GET /api/app/logs": h.appLogs, "GET /api/app/recordings/{id}": h.appRecording, "GET /api/app/preferences": h.preferences, "PATCH /api/app/preferences": h.preferences, "GET /ws": h.secureTerminal} {
		mux.HandleFunc(pattern, h.require(false, fn))
	}
	for pattern, fn := range map[string]http.HandlerFunc{"GET /api/admin/users": h.adminUsers, "POST /api/admin/users": h.adminUsers, "GET /api/admin/users/{id}": h.adminUser, "PATCH /api/admin/users/{id}": h.adminUser, "POST /api/admin/users/{id}/reset-password": h.adminUser, "GET /api/admin/targets": h.adminTargets, "POST /api/admin/targets": h.adminTargets, "GET /api/admin/targets/{id}": h.adminTarget, "PATCH /api/admin/targets/{id}": h.adminTarget, "GET /api/admin/targets/{target}/accounts": h.adminAccounts, "POST /api/admin/targets/{target}/accounts": h.adminAccounts, "GET /api/admin/targets/{target}/accounts/{id}": h.adminAccount, "PATCH /api/admin/targets/{target}/accounts/{id}": h.adminAccount, "GET /api/admin/access-grants": h.adminGrants, "PUT /api/admin/users/{user}/access-grants/{account}": h.adminGrants, "DELETE /api/admin/users/{user}/access-grants/{account}": h.adminGrants, "GET /api/admin/sessions": h.adminSessions, "DELETE /api/admin/sessions/{id}": h.endSession, "GET /api/admin/shares": h.adminShares,
		"DELETE /api/admin/shares/{share}": h.deleteShare, "GET /api/admin/logs": h.appLogs, "GET /api/admin/recordings/{id}": h.appRecording, "GET /api/admin/audit-events": h.auditEvents, "GET /api/admin/settings": h.settings, "PATCH /api/admin/settings": h.settings, "GET /api/admin/health": h.adminHealth} {
		mux.HandleFunc(pattern, h.require(true, fn))
	}
	gone := func(w http.ResponseWriter, r *http.Request) {
		apiError(w, 410, "この API は廃止されました。ログインしてご利用ください", "LEGACY_API_REMOVED")
	}
	for _, path := range []string{"/api/connect", "/api/session", "/api/session/", "/api/shared/", "/api/sessions", "/api/sessions/", "/api/logs", "/api/recordings", "/api/recordings/"} {
		mux.HandleFunc(path, gone)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { missing(w) })
	return loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		mux.ServeHTTP(w, r)
	}))
}
func adminPath(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/api/admin/") }

func decodePatch(w http.ResponseWriter, r *http.Request, out any) bool {
	var patch map[string]json.RawMessage
	if !decode(w, r, &patch) {
		return false
	}
	if patch == nil {
		invalid(w, errors.New("JSON object required"))
		return false
	}
	base, e := json.Marshal(out)
	if e != nil {
		unavailable(w, e)
		return false
	}
	var merged map[string]json.RawMessage
	if e = json.Unmarshal(base, &merged); e != nil {
		unavailable(w, e)
		return false
	}
	for key, v := range patch {
		merged[key] = v
	}
	b, e := json.Marshal(merged)
	if e != nil {
		invalid(w, e)
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(out); e != nil {
		invalid(w, e)
		return false
	}
	return true
}

func (h *Handler) clientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip, e := netip.ParseAddr(host)
	if e != nil {
		return host
	}
	for _, cidr := range h.config.TrustedProxyCIDRs {
		p, e := netip.ParsePrefix(cidr)
		if e == nil && p.Contains(ip) {
			if real, e := netip.ParseAddr(r.Header.Get("X-Real-IP")); e == nil {
				return real.Unmap().String()
			}
		}
	}
	return ip.Unmap().String()
}
