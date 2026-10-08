package api

import (
	"net/http"

	"github.com/nagayon-935/conduit/internal/control"
)

func (h *Handler) adminGrants(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		h.objects(w, "grant", "")
		return
	}
	uid, aid := r.PathValue("user"), r.PathValue("account")
	var g control.Grant
	if r.Method == "PUT" {
		if !decode(w, r, &g) {
			return
		}
	}
	g.UserID, g.AccountID = uid, aid
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	var a control.Account
	if _, e := h.control.User(uid); e != nil {
		missing(w)
		return
	}
	if h.control.Get("account", aid, &a) != nil {
		missing(w)
		return
	}
	var e error
	if r.Method == "DELETE" {
		e = h.control.Delete(who(r).User.ID, "grant", uid+":"+aid, "")
	} else {
		e = h.control.Save(who(r).User.ID, "grant", uid+":"+aid, uid, g)
	}
	if e != nil {
		unavailable(w, e)
		return
	}
	h.cancelPendingLocked()
	h.enforceLocked()
	w.WriteHeader(204)
}
