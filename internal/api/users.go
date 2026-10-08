package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/nagayon-935/conduit/internal/control"
)

func (h *Handler) adminUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, e := h.control.Users()
		if e != nil {
			unavailable(w, e)
			return
		}
		writeJSON(w, 200, v)
		return
	}
	var req struct {
		Login    string `json:"login"`
		Name     string `json:"display_name"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	hash, e := control.HashPassword(req.Password)
	if e != nil {
		invalid(w, e)
		return
	}
	u := control.User{ID: control.Random(), Login: req.Login, Name: req.Name, Role: req.Role, Enabled: true, MustChange: true, Hash: hash}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	if e = h.control.SaveUser(who(r).User.ID, u); e != nil {
		invalid(w, e)
		return
	}
	writeJSON(w, 201, u)
}

func (h *Handler) adminUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if r.Method == "GET" {
		u, e := h.control.User(id)
		if e != nil {
			missing(w)
			return
		}
		writeJSON(w, 200, u)
		return
	}
	var req struct {
		Name     *string `json:"display_name"`
		Role     *string `json:"role"`
		Enabled  *bool   `json:"enabled"`
		Password string  `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	var hash string
	var e error
	reset := strings.HasSuffix(r.URL.Path, "/reset-password")
	if reset {
		hash, e = control.HashPassword(req.Password)
		if e != nil {
			invalid(w, e)
			return
		}
	} else if req.Password != "" {
		invalid(w, errors.New("use reset-password"))
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	u, e := h.control.User(id)
	if e != nil {
		missing(w)
		return
	}
	previous := u
	if req.Name != nil {
		u.Name = *req.Name
	}
	if req.Role != nil {
		u.Role = *req.Role
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	if reset {
		u.Hash = hash
		u.MustChange = true
	}
	if e = h.control.SaveUser(who(r).User.ID, u); e != nil {
		invalid(w, e)
		return
	}
	if reset || u.Role != previous.Role || u.Enabled != previous.Enabled {
		h.revokeUserLocked(id, reset || !u.Enabled)
	}
	h.cancelPendingLocked()
	writeJSON(w, 200, u)
}
