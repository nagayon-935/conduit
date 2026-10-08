package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/nagayon-935/conduit/internal/control"
)

func (h *Handler) appLogs(w http.ResponseWriter, r *http.Request) {
	owner := who(r).User.ID
	if adminPath(r) {
		owner = ""
	}
	cursorID := r.URL.Query().Get("cursor")
	if cursorID != "" {
		if _, e := h.control.Log(cursorID, owner); e != nil {
			if e == sql.ErrNoRows {
				missing(w)
			} else {
				unavailable(w, e)
			}
			return
		}
	}
	v, e := h.control.Logs(owner, cursorID)
	if e != nil {
		unavailable(w, e)
		return
	}
	cursor := ""
	if len(v) == 50 {
		cursor = v[len(v)-1].ID
	}
	writeJSON(w, 200, map[string]any{"items": v, "next_cursor": cursor})
}

func (h *Handler) auditEvents(w http.ResponseWriter, r *http.Request) {
	cursor, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	v, e := h.control.Audit(cursor)
	if e != nil {
		unavailable(w, e)
		return
	}
	writeJSON(w, 200, v)
}

func (h *Handler) adminHealth(w http.ResponseWriter, r *http.Request) {
	if e := h.control.DB.Ping(); e != nil {
		unavailable(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "active_sessions": len(h.sessions.List()), "storage": "sqlite", "recording_enabled": h.policy().Recording})
}

func (h *Handler) adminShares(w http.ResponseWriter, r *http.Request) {
	raw, e := h.control.Objects("share", "")
	if e != nil {
		unavailable(w, e)
		return
	}
	out := []control.Share{}
	for _, b := range raw {
		var s control.Share
		if e = json.Unmarshal(b, &s); e != nil {
			unavailable(w, e)
			return
		}
		if h.shareActive(s.ID) {
			out = append(out, s)
		}
	}
	writeJSON(w, 200, out)
}

// A configured route authorizes transit; it does not grant an interactive login
// on the jump account. Direct use still requires that account's own grant.
