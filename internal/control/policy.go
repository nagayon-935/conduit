package control

import "database/sql"

type Policy struct {
	Revision         int   `json:"revision"`
	GraceMinutes     int   `json:"reconnect_grace_minutes"`
	SSHIdleMinutes   int   `json:"ssh_idle_minutes"`
	LoginHours       int   `json:"login_absolute_hours"`
	LoginIdleMinutes int   `json:"login_idle_minutes"`
	ShareTTL         int   `json:"share_ttl_minutes"`
	Recording        bool  `json:"recording_enabled"`
	RecordingDays    int   `json:"recording_retention_days"`
	LogDays          int   `json:"log_retention_days"`
	AuditDays        int   `json:"audit_retention_days"`
	RecordingBytes   int64 `json:"recording_max_bytes"`
}

func DefaultPolicy() Policy {
	return Policy{Revision: 1, GraceMinutes: 15, SSHIdleMinutes: 30, LoginHours: 8, LoginIdleMinutes: 60, ShareTTL: 60, RecordingDays: 30, LogDays: 90, AuditDays: 180, RecordingBytes: 10 * 1024 * 1024 * 1024}
}
func (s *Store) Policy() (Policy, error) {
	p := DefaultPolicy()
	err := s.Get("settings", "policy", &p)
	if err == sql.ErrNoRows {
		err = nil
	}
	return p, err
}
