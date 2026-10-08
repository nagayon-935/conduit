package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nagayon-935/conduit/internal/control"
)

func (h *Handler) policy() control.Policy {
	p := control.DefaultPolicy()
	p.GraceMinutes = int(h.config.GracePeriod / time.Minute)
	p.SSHIdleMinutes = int(h.config.IdleTimeout / time.Minute)
	p.Recording = h.config.RecordingEnabled
	if p.GraceMinutes == 0 {
		p.GraceMinutes = 15
	}
	_ = h.control.Get("settings", "policy", &p)
	return p
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		writeJSON(w, 200, h.policy())
		return
	}
	p := h.policy()
	p.Revision = 0
	if !decodePatch(w, r, &p) {
		return
	}
	if p.Revision < 1 || p.GraceMinutes < 1 || p.GraceMinutes > 120 || p.SSHIdleMinutes < 0 || p.SSHIdleMinutes > 1440 || p.LoginHours < 1 || p.LoginHours > 24 || p.LoginIdleMinutes < 5 || p.LoginIdleMinutes > p.LoginHours*60 || p.RecordingBytes > 1024*1024*1024*1024 || p.ShareTTL < 1 || p.ShareTTL > 1440 || p.RecordingDays < 1 || p.LogDays < 1 || p.AuditDays < 1 || p.RecordingDays > 3650 || p.LogDays > 3650 || p.AuditDays > 3650 || p.RecordingBytes < 1024*1024 {
		invalid(w, fmt.Errorf("invalid retention policy"))
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	old := h.policy()
	if p.Revision != old.Revision {
		apiError(w, 409, "別の管理者が設定を更新しました。再読み込みしてください", "REVISION_CONFLICT")
		return
	}
	p.Revision++
	if e := h.control.Save(who(r).User.ID, "settings", "policy", "", p); e != nil {
		unavailable(w, e)
		return
	}
	h.cancelPendingLocked()
	for _, info := range h.sessions.List() {
		if old.Recording != p.Recording {
			h.terminateLocked(info.ID, "recording_policy_changed")
			continue
		}
		if s, e := h.sessions.Get(info.ID); e == nil {
			s.ConfigureLimits(time.Duration(p.GraceMinutes)*time.Minute, time.Duration(p.SSHIdleMinutes)*time.Minute)
		}
	}
	h.enforceLocked()
	writeJSON(w, 200, p)
}

type preferences struct {
	Theme     string   `json:"theme"`
	FontSize  int      `json:"font_size"`
	Favorites []string `json:"favorites"`
}

func (h *Handler) preferences(w http.ResponseWriter, r *http.Request) {
	i := who(r)
	p := preferences{Theme: "tokyo-night", FontSize: 14, Favorites: []string{}}
	if r.Method == "GET" {
		e := h.control.Get("preferences", i.User.ID, &p)
		if e != nil && e != sql.ErrNoRows {
			unavailable(w, e)
			return
		}
		writeJSON(w, 200, p)
		return
	}
	if !decode(w, r, &p) {
		return
	}
	if len(p.Theme) > 64 || p.FontSize < 8 || p.FontSize > 32 || len(p.Favorites) > 200 {
		invalid(w, errors.New("invalid preferences"))
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	defer h.access.Unlock()
	if e := h.control.Save(i.User.ID, "preferences", i.User.ID, i.User.ID, p); e != nil {
		unavailable(w, e)
		return
	}
	writeJSON(w, 200, p)
}
