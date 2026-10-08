package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/control"
	"github.com/nagayon-935/conduit/internal/tunnel"
)

type socketAccess struct {
	LoginHash, UserID, SessionID, ShareID string
	WS                                    *websocket.Conn
}
type wsTicket struct {
	Access  socketAccess
	Expires time.Time
}

func (h *Handler) shareActive(id string) bool {
	v, err := h.control.Share(id)
	if err != nil || time.Now().Unix() >= v.Expires {
		return false
	}
	if _, _, err = h.control.Login(v.LoginHash); err != nil {
		return false
	}
	s, err := h.sessions.Get(v.SessionID)
	return err == nil && !s.IsExpired()
}
func (h *Handler) sharedSession(uid, id string) (control.Share, error) {
	v, e := h.control.Share(id)
	if e != nil || time.Now().Unix() >= v.Expires || !slices.Contains(v.Recipients, uid) {
		return v, errors.New("share unavailable")
	}
	if _, _, e = h.control.Login(v.LoginHash); e != nil {
		return v, e
	}
	s, e := h.sessions.Get(v.SessionID)
	if e != nil || s.IsExpired() {
		return v, errors.New("session ended")
	}
	a, _, e := h.account(uid, s.TargetAccountID, true)
	if e != nil || !a.Sharing {
		return v, errors.New("view not allowed")
	}
	return v, nil
}
func (h *Handler) socketAllowed(a socketAccess) bool {
	_, u, e := h.control.Login(a.LoginHash)
	if e != nil || u.ID != a.UserID || u.MustChange {
		return false
	}
	if a.ShareID != "" {
		v, e := h.sharedSession(u.ID, a.ShareID)
		return e == nil && v.SessionID == a.SessionID
	}
	s, e := h.sessions.Get(a.SessionID)
	if e != nil || s.OwnerUserID != u.ID || s.IsExpired() {
		return false
	}
	_, _, e = h.account(u.ID, s.TargetAccountID, false)
	return e == nil
}
func (h *Handler) issueTicket(w http.ResponseWriter, r *http.Request) {
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	i := who(r)
	a := socketAccess{LoginHash: i.Login.Hash, UserID: i.User.ID, SessionID: r.PathValue("id"), ShareID: r.PathValue("share")}
	if a.ShareID != "" {
		v, e := h.sharedSession(i.User.ID, a.ShareID)
		if e != nil {
			missing(w)
			return
		}
		a.SessionID = v.SessionID
	}
	if !h.socketAllowed(a) {
		missing(w)
		return
	}
	now := time.Now()
	for k, t := range h.tickets {
		if now.After(t.Expires) {
			delete(h.tickets, k)
		}
	}
	if len(h.tickets) >= 4096 {
		apiError(w, 429, "接続を再試行してください", "RATE_LIMITED")
		return
	}
	ticket := control.Random()
	exp := now.Add(30 * time.Second)
	h.tickets[control.TokenHash(ticket)] = wsTicket{a, exp}
	writeJSON(w, 201, map[string]any{"ticket": ticket, "expires_at": exp.Unix()})
}
func (h *Handler) secureTerminal(w http.ResponseWriter, r *http.Request) {
	if !h.originOK(r) {
		apiError(w, 403, "Origin is required", "ORIGIN_FAILED")
		return
	}
	q := r.URL.Query()
	if len(q) != 1 || len(q["ticket"]) != 1 || q.Get("ticket") == "" {
		apiError(w, 410, "Use an authenticated one-time ticket", "LEGACY_API_REMOVED")
		return
	}
	i := who(r)
	key := control.TokenHash(q.Get("ticket"))
	h.access.Lock()
	t, ok := h.tickets[key]
	if !ok || t.Access.LoginHash != i.Login.Hash || time.Now().After(t.Expires) || !h.socketAllowed(t.Access) {
		h.access.Unlock()
		apiError(w, 403, "接続チケットが無効です", "INVALID_TICKET")
		return
	}
	delete(h.tickets, key)
	upgrader := h.upgrader
	upgrader.CheckOrigin = h.originOK
	upgrader.HandshakeTimeout = 5 * time.Second
	ws, e := upgrader.Upgrade(w, r, nil)
	if e != nil {
		h.access.Unlock()
		return
	}
	ws.SetReadLimit(64 * 1024)
	id := generateConnID()
	s, removed, e := h.sessions.Attach(t.Access.SessionID, id, ws, t.Access.ShareID != "")
	if e != nil {
		h.access.Unlock()
		ws.Close()
		return
	}
	t.Access.WS = ws
	h.sockets[id] = t.Access
	h.access.Unlock()
	defer func() { ws.Close(); s.RemoveWebSocket(id); h.access.Lock(); delete(h.sockets, id); h.access.Unlock() }()
	cfg := tunnel.DefaultPumpConfig()
	cfg.Authorize = func(activity bool) bool {
		h.access.Lock()
		defer h.access.Unlock()
		if !h.socketAllowed(t.Access) {
			return false
		}
		if activity && t.Access.ShareID == "" {
			return h.control.Touch(t.Access.LoginHash) == nil
		}
		return true
	}
	s.StartOnce(func() { tunnel.StartSessionPumps(s.Context(), s, tunnel.DefaultPumpConfig()) })
	tunnel.StartConnectionPump(id, ws, s, cfg)
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-s.Done():
			sendWSExit(s.GetSafeConn(id))
			return
		case <-removed:
			return
		case <-timer.C:
			h.access.Lock()
			valid := h.socketAllowed(t.Access)
			h.access.Unlock()
			if !valid {
				_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4001, "authorization expired"), time.Now().Add(time.Second))
				return
			}
		}
	}
}
func (h *Handler) shareCandidates(w http.ResponseWriter, r *http.Request) {
	s, e := h.sessionFor(r)
	if e != nil {
		missing(w)
		return
	}
	var a control.Account
	if h.control.Get("account", s.TargetAccountID, &a) != nil || !a.Sharing {
		missing(w)
		return
	}
	users, e := h.control.Users()
	if e != nil {
		unavailable(w, e)
		return
	}
	out := []map[string]string{}
	for _, u := range users {
		if !u.Enabled || u.MustChange || u.ID == who(r).User.ID {
			continue
		}
		if _, _, e = h.account(u.ID, a.ID, true); e == nil {
			out = append(out, map[string]string{"id": u.ID, "display_name": u.Name})
		}
	}
	writeJSON(w, 200, out)
}
func (h *Handler) shares(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s, e := h.sessionFor(r)
		if e != nil {
			missing(w)
			return
		}
		raw, e := h.control.Objects("share", s.LogID)
		if e != nil {
			unavailable(w, e)
			return
		}
		out := []control.Share{}
		for _, b := range raw {
			var v control.Share
			if e = json.Unmarshal(b, &v); e != nil {
				unavailable(w, e)
				return
			}
			if h.shareActive(v.ID) {
				out = append(out, v)
			}
		}
		writeJSON(w, 200, out)
		return
	}
	var req struct {
		Recipients []string `json:"recipient_user_ids"`
		TTL        int      `json:"ttl_minutes"`
	}
	if !decode(w, r, &req) {
		return
	}
	if len(req.Recipients) == 0 || len(req.Recipients) > 200 {
		invalid(w, errors.New("共有するユーザーを選択してください"))
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	s, e := h.sessionFor(r)
	if e != nil {
		missing(w)
		return
	}
	var a control.Account
	if h.control.Get("account", s.TargetAccountID, &a) != nil || !a.Sharing {
		missing(w)
		return
	}
	for _, uid := range req.Recipients {
		u, e := h.control.User(uid)
		if e != nil || !u.Enabled || u.MustChange {
			missing(w)
			return
		}
		if _, _, e = h.account(uid, a.ID, true); e != nil {
			missing(w)
			return
		}
	}
	ttl := h.policy().ShareTTL
	if req.TTL > 0 && req.TTL <= ttl {
		ttl = req.TTL
	}
	v := control.Share{ID: control.Random(), SessionID: s.LogID, Owner: who(r).User.ID, LoginHash: who(r).Login.Hash, Recipients: req.Recipients, Expires: time.Now().Add(time.Duration(ttl) * time.Minute).Unix()}
	if e = h.control.SaveShare(v.Owner, v); e != nil {
		unavailable(w, e)
		return
	}
	writeJSON(w, 201, map[string]any{"share": v, "url": "/app/shared/" + v.ID})
}
func (h *Handler) deleteShare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if adminPath(r) && req.Reason == "" {
		invalid(w, errors.New("取り消し理由を入力してください"))
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	v, e := h.control.Share(r.PathValue("share"))
	if e != nil || (!adminPath(r) && (v.Owner != who(r).User.ID || v.SessionID != r.PathValue("id"))) {
		missing(w)
		return
	}
	if e = h.control.Delete(who(r).User.ID, "share", v.ID, req.Reason); e != nil {
		unavailable(w, e)
		return
	}
	h.enforceLocked()
	w.WriteHeader(204)
}
func (h *Handler) shared(w http.ResponseWriter, r *http.Request) {
	v, e := h.sharedSession(who(r).User.ID, r.PathValue("share"))
	if e != nil {
		missing(w)
		return
	}
	s, e := h.sessions.Get(v.SessionID)
	if e != nil {
		missing(w)
		return
	}
	creator, e := h.control.User(v.Owner)
	if e != nil {
		unavailable(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"share": v, "session": s.Info(), "creator_name": creator.Name})
}
