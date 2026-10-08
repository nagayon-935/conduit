package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (h *Handler) Close() {
	h.access.Lock()
	defer h.access.Unlock()
	h.cancelPendingLocked()
	for _, a := range h.sockets {
		a.WS.Close()
	}
	for _, s := range h.sessions.List() {
		h.terminateLocked(s.ID, "server_shutdown")
	}
}
func (h *Handler) StartMaintenance(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.access.Lock()
				h.enforceLocked()
				h.access.Unlock()
				if e := h.retain(); e != nil {
					slog.Error("retention failed", "error", e)
				}
			}
		}
	}()
}
func (h *Handler) retain() error {
	p := h.policy()
	now := time.Now()
	rows, e := h.control.DB.Query("SELECT id,path,started FROM ssh_session_records WHERE ended IS NOT NULL AND path<>'' ORDER BY started ASC")
	if e != nil {
		return e
	}
	type cast struct {
		ID, Path string
		At       int64
		Size     int64
	}
	files := []cast{}
	var total int64
	for rows.Next() {
		var c cast
		if e = rows.Scan(&c.ID, &c.Path, &c.At); e != nil {
			rows.Close()
			return e
		}
		if f, e := os.Stat(c.Path); e == nil {
			c.Size = f.Size()
		}
		total += c.Size
		files = append(files, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	sort.Slice(files, func(i, j int) bool { return files[i].At < files[j].At })
	base, e := filepath.Abs(h.config.RecordingDir)
	if e != nil {
		return e
	}
	for _, c := range files {
		if c.At >= now.Add(-time.Duration(p.RecordingDays)*24*time.Hour).Unix() && total <= p.RecordingBytes {
			continue
		}
		path, e := filepath.Abs(c.Path)
		if e != nil || !strings.HasPrefix(path, base+string(filepath.Separator)) {
			continue
		}
		if e = os.Remove(path); e != nil && !os.IsNotExist(e) {
			return e
		}
		if _, e = h.control.DB.Exec("UPDATE ssh_session_records SET path='' WHERE id=?", c.ID); e != nil {
			return e
		}
		total -= c.Size
	}
	// Remove casts before deleting their logs, so retention cannot orphan files.
	old, e := h.control.DB.Query("SELECT id,path FROM ssh_session_records WHERE ended IS NOT NULL AND started<? AND path<>''", now.Add(-time.Duration(p.LogDays)*24*time.Hour).Unix())
	if e != nil {
		return e
	}
	paths := []cast{}
	for old.Next() {
		var c cast
		if e = old.Scan(&c.ID, &c.Path); e != nil {
			old.Close()
			return e
		}
		paths = append(paths, c)
	}
	e = old.Err()
	old.Close()
	if e != nil {
		return e
	}
	for _, c := range paths {
		path, e := filepath.Abs(c.Path)
		if e != nil || !strings.HasPrefix(path, base+string(filepath.Separator)) {
			continue
		}
		if e = os.Remove(path); e != nil && !os.IsNotExist(e) {
			return e
		}
		if _, e = h.control.DB.Exec("UPDATE ssh_session_records SET path='' WHERE id=?", c.ID); e != nil {
			return e
		}
	}
	if _, e = h.control.DB.Exec("DELETE FROM ssh_session_records WHERE ended IS NOT NULL AND started<? AND path=''", now.Add(-time.Duration(p.LogDays)*24*time.Hour).Unix()); e != nil {
		return e
	}
	if _, e = h.control.DB.Exec("DELETE FROM audit_events WHERE at<?", now.Add(-time.Duration(p.AuditDays)*24*time.Hour).Unix()); e != nil {
		return e
	}
	if _, e = h.control.DB.Exec("DELETE FROM login_sessions WHERE created<? OR active<?", now.Unix()-int64(p.LoginHours)*3600, now.Unix()-int64(p.LoginIdleMinutes)*60); e != nil {
		return e
	}
	raw, e := h.control.Objects("share", "")
	if e != nil {
		return e
	}
	for _, b := range raw {
		var v struct {
			ID string `json:"id"`
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return e
		}
		if !h.shareActive(v.ID) {
			if _, e = h.control.DB.Exec("DELETE FROM control_objects WHERE kind='share' AND id=?", v.ID); e != nil {
				return e
			}
		}
	}
	entries, e := os.ReadDir(h.config.RecordingDir)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".cast") {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if now.Sub(info.ModTime()) < 24*time.Hour {
			continue
		}
		path := filepath.Join(h.config.RecordingDir, entry.Name())
		var n int
		if e = h.control.DB.QueryRow("SELECT count(*) FROM ssh_session_records WHERE path=?", path).Scan(&n); e != nil {
			return e
		}
		if n == 0 {
			if e = os.Remove(path); e != nil {
				return e
			}
		}
	}
	return nil
}
