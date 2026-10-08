package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/nagayon-935/conduit/internal/control"
	"github.com/nagayon-935/conduit/internal/session"
)

func (h *Handler) beginChange(w http.ResponseWriter, r *http.Request) bool {
	h.access.Lock()
	i := who(r)
	_, u, e := h.control.Login(i.Login.Hash)
	if e != nil || u.Version != i.User.Version || (adminPath(r) && u.Role != "admin") {
		h.access.Unlock()
		apiError(w, 401, "ログインし直してください", "UNAUTHENTICATED")
		return false
	}
	return true
}

func (h *Handler) account(uid, id string, view bool) (control.Account, control.Target, error) {
	var a control.Account
	var t control.Target
	g, e := h.control.Grant(uid, id)
	if e != nil || (!view && !g.Connect) || (view && !g.View) {
		return a, t, errors.New("no grant")
	}
	if e = h.control.Get("account", id, &a); e != nil || !a.Enabled {
		return a, t, errors.New("account unavailable")
	}
	if e = h.control.Get("target", a.TargetID, &t); e != nil || !t.Enabled {
		return a, t, errors.New("target unavailable")
	}
	return a, t, nil
}

func (h *Handler) sessionFor(r *http.Request) (*session.Session, error) {
	s, e := h.sessions.Get(r.PathValue("id"))
	if e != nil || s.OwnerUserID != who(r).User.ID || s.IsExpired() {
		return nil, errors.New("not found")
	}
	_, _, e = h.account(who(r).User.ID, s.TargetAccountID, false)
	return s, e
}

func (h *Handler) appSessions(w http.ResponseWriter, r *http.Request) {
	out := []session.SessionInfo{}
	for _, s := range h.sessions.List() {
		if s.OwnerUserID == who(r).User.ID && s.State != "terminated" {
			out = append(out, s)
		}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) adminSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, h.sessions.List())
}

func (h *Handler) appSession(w http.ResponseWriter, r *http.Request) {
	s, e := h.sessionFor(r)
	if e != nil {
		missing(w)
		return
	}
	writeJSON(w, 200, s.Info())
}

func (h *Handler) endSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if adminPath(r) && strings.TrimSpace(req.Reason) == "" {
		invalid(w, errors.New("終了理由を入力してください"))
		return
	}
	if len(req.Reason) > 1024 {
		invalid(w, errors.New("reason too long"))
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	s, e := h.sessions.Get(r.PathValue("id"))
	if e != nil || (!adminPath(r) && s.OwnerUserID != who(r).User.ID) {
		missing(w)
		return
	}
	reason := req.Reason
	if reason == "" {
		reason = "owner_terminated"
	}
	if e = h.control.Mutation(who(r).User.ID, "session.terminate", s.LogID, reason, func(tx *sql.Tx) error {
		_, e := tx.Exec("UPDATE ssh_session_records SET ended=?,reason=? WHERE id=?", time.Now().Unix(), reason, s.LogID)
		return e
	}); e != nil {
		unavailable(w, e)
		return
	}
	_ = h.sessions.TerminateWithReason(s.Token, reason)
	h.enforceLocked()
	w.WriteHeader(204)
}

func (h *Handler) terminateOwner(uid, reason string) {
	for _, s := range h.sessions.List() {
		if s.OwnerUserID == uid {
			h.terminateLocked(s.ID, reason)
		}
	}
}

func (h *Handler) terminateLocked(id, reason string) {
	if e := h.control.EndLog(id, reason); e != nil { /* Close anyway: authorization revocation must fail closed. */
	}
	_ = h.sessions.TerminateWithReason(id, reason)
}

func (h *Handler) terminateAccountLocked(aid, reason string) {
	raw, _ := h.control.Objects("target", "")
	affected := map[string]bool{}
	for _, b := range raw {
		var t control.Target
		_ = json.Unmarshal(b, &t)
		if t.JumpAccountID == aid {
			affected[t.ID] = true
		}
	}
	for _, s := range h.sessions.List() {
		var a control.Account
		_ = h.control.Get("account", s.TargetAccountID, &a)
		if s.TargetAccountID == aid || affected[a.TargetID] {
			h.terminateLocked(s.ID, reason)
		}
	}
}

func (h *Handler) terminateTargetLocked(tid, reason string) {
	raw, _ := h.control.Objects("account", tid)
	for _, b := range raw {
		var a control.Account
		_ = json.Unmarshal(b, &a)
		h.terminateAccountLocked(a.ID, reason)
	}
}

func (h *Handler) cancelPendingLocked() {
	for id, cancel := range h.pending {
		cancel()
		delete(h.pending, id)
	}
}

func (h *Handler) enforceLocked() {
	for _, s := range h.sessions.List() {
		u, e := h.control.User(s.OwnerUserID)
		a, t, e2 := h.account(s.OwnerUserID, s.TargetAccountID, false)
		invalid := e != nil || !u.Enabled || e2 != nil
		if !invalid && t.JumpAccountID != "" {
			_, _, e = h.jump(t)
			invalid = e != nil
		}
		_ = a
		if invalid {
			h.terminateLocked(s.ID, "access_revoked")
		}
	}
	for id, a := range h.sockets {
		if !h.socketAllowed(a) {
			a.WS.Close()
			delete(h.sockets, id)
		}
	}
	for k, t := range h.tickets {
		if !h.socketAllowed(t.Access) || time.Now().After(t.Expires) {
			delete(h.tickets, k)
		}
	}
}

func (h *Handler) revokeLoginLocked(hash string) {
	raw, e := h.control.Objects("share", "")
	if e == nil {
		for _, b := range raw {
			var v struct {
				ID   string `json:"id"`
				Hash string `json:"creator_login_hash"`
			}
			if json.Unmarshal(b, &v) == nil && v.Hash == hash {
				_ = h.control.Delete("system", "share", v.ID, "creator_logged_out")
			}
		}
	}
	for id, a := range h.sockets {
		if a.LoginHash == hash {
			a.WS.Close()
			delete(h.sockets, id)
		}
	}
	h.enforceLocked()
}

func (h *Handler) revokeUserLocked(uid string, terminate bool) {
	_ = h.control.RevokeUser(uid)
	raw, _ := h.control.Objects("share", "")
	for _, b := range raw {
		var v struct {
			control.Share
			Hash string `json:"creator_login_hash"`
		}
		if json.Unmarshal(b, &v) == nil && v.Owner == uid {
			_ = h.control.Delete("system", "share", v.ID, "creator_login_revoked")
		}
	}
	if terminate {
		h.terminateOwner(uid, "user_disabled_or_password_reset")
	}
	h.enforceLocked()
}
