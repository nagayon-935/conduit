package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/nagayon-935/conduit/internal/control"
)

func (h *Handler) adminTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		h.objects(w, "target", "")
		return
	}
	var t control.Target
	if !decode(w, r, &t) {
		return
	}
	t.ID = control.Random()
	h.saveTarget(w, r, t)
}

func (h *Handler) adminTarget(w http.ResponseWriter, r *http.Request) {
	var t control.Target
	if e := h.control.Get("target", r.PathValue("id"), &t); e != nil {
		missing(w)
		return
	}
	if r.Method == "GET" {
		writeJSON(w, 200, t)
		return
	}
	req := t
	if !decodePatch(w, r, &req) {
		return
	}
	req.ID = t.ID
	h.saveTarget(w, r, req)
}

func (h *Handler) saveTarget(w http.ResponseWriter, r *http.Request, t control.Target) {
	t.Host = strings.TrimSpace(t.Host)
	if t.Name == "" || len(t.Name) > 256 || len(t.Host) > 253 || t.Host == "" || strings.ContainsAny(t.Host, " /\\\t\r\n") || t.Port < 1 || t.Port > 65535 || len(t.Environment) > 128 {
		invalid(w, errors.New("name, host and port (1–65535) are required"))
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	if t.JumpAccountID != "" {
		raw, e := h.control.Objects("target", "")
		if e != nil {
			unavailable(w, e)
			return
		}
		for _, b := range raw {
			var other control.Target
			if e = json.Unmarshal(b, &other); e != nil {
				unavailable(w, e)
				return
			}
			if other.ID == t.ID || other.JumpAccountID == "" {
				continue
			}
			var jump control.Account
			if h.control.Get("account", other.JumpAccountID, &jump) == nil && jump.TargetID == t.ID {
				invalid(w, errors.New("a target used as a jump cannot itself have a jump"))
				return
			}
		}
		var a control.Account
		var jt control.Target
		if h.control.Get("account", t.JumpAccountID, &a) != nil || h.control.Get("target", a.TargetID, &jt) != nil || jt.ID == t.ID || jt.JumpAccountID != "" {
			invalid(w, errors.New("jump account must refer to another target without a jump"))
			return
		}
	}
	if e := h.control.Save(who(r).User.ID, "target", t.ID, "", t); e != nil {
		unavailable(w, e)
		return
	}
	h.cancelPendingLocked()
	h.terminateTargetLocked(t.ID, "target_changed")
	h.enforceLocked()
	writeJSON(w, 200, t)
}

func (h *Handler) adminAccounts(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("target")
	var t control.Target
	if h.control.Get("target", target, &t) != nil {
		missing(w)
		return
	}
	if r.Method == "GET" {
		h.objects(w, "account", target)
		return
	}
	var a control.Account
	if !decode(w, r, &a) {
		return
	}
	a.ID = control.Random()
	a.TargetID = target
	h.saveAccount(w, r, a)
}

func (h *Handler) adminAccount(w http.ResponseWriter, r *http.Request) {
	var a control.Account
	if h.control.Get("account", r.PathValue("id"), &a) != nil || a.TargetID != r.PathValue("target") {
		missing(w)
		return
	}
	if r.Method == "GET" {
		writeJSON(w, 200, a)
		return
	}
	req := a
	if !decodePatch(w, r, &req) {
		return
	}
	req.ID, req.TargetID = a.ID, a.TargetID
	h.saveAccount(w, r, req)
}

func (h *Handler) saveAccount(w http.ResponseWriter, r *http.Request, a control.Account) {
	if a.Username == "" || len(a.Username) > 128 || strings.ContainsAny(a.Username, "\r\n\x00") || (a.AuthType != "vault" && a.AuthType != "password" && a.AuthType != "pubkey") {
		invalid(w, errors.New("SSH username and auth_type (vault/password/pubkey) are required"))
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	raw, e := h.control.Objects("account", a.TargetID)
	if e != nil {
		unavailable(w, e)
		return
	}
	for _, b := range raw {
		var existing control.Account
		if e = json.Unmarshal(b, &existing); e != nil {
			unavailable(w, e)
			return
		}
		if existing.ID != a.ID && existing.Username == a.Username {
			apiError(w, 409, "この接続先の SSH アカウントは登録済みです", "ACCOUNT_EXISTS")
			return
		}
	}
	if e := h.control.Save(who(r).User.ID, "account", a.ID, a.TargetID, a); e != nil {
		unavailable(w, e)
		return
	}
	h.cancelPendingLocked()
	h.terminateAccountLocked(a.ID, "account_changed")
	h.enforceLocked()
	writeJSON(w, 200, a)
}

func (h *Handler) objects(w http.ResponseWriter, kind, parent string) {
	v, e := h.control.Objects(kind, parent)
	if e != nil {
		unavailable(w, e)
		return
	}
	writeJSON(w, 200, v)
}

func (h *Handler) appTargets(w http.ResponseWriter, r *http.Request) {
	i := who(r)
	raw, e := h.control.Objects("grant", i.User.ID)
	if e != nil {
		unavailable(w, e)
		return
	}
	out := []any{}
	for _, b := range raw {
		var g control.Grant
		if e = json.Unmarshal(b, &g); e != nil {
			unavailable(w, e)
			return
		}
		if !g.Connect {
			continue
		}
		a, t, e := h.account(i.User.ID, g.AccountID, false)
		if e != nil {
			continue
		}
		item := map[string]any{"target": t, "account": a}
		if t.JumpAccountID != "" {
			ja, jt, e := h.jump(t)
			if e != nil {
				continue
			}
			item["jump_account"] = ja
			item["jump_target"] = jt
		}
		out = append(out, item)
	}
	writeJSON(w, 200, out)
}

func (h *Handler) jump(t control.Target) (control.Account, control.Target, error) {
	var a control.Account
	var jt control.Target
	if e := h.control.Get("account", t.JumpAccountID, &a); e != nil || !a.Enabled {
		return a, jt, errors.New("jump account unavailable")
	}
	if e := h.control.Get("target", a.TargetID, &jt); e != nil || !jt.Enabled || jt.JumpAccountID != "" || jt.ID == t.ID {
		return a, jt, errors.New("jump route unavailable")
	}
	return a, jt, nil
}
