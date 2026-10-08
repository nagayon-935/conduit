package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nagayon-935/conduit/internal/control"
	"github.com/nagayon-935/conduit/internal/recording"
	"github.com/nagayon-935/conduit/internal/session"
	"github.com/nagayon-935/conduit/internal/sshconn"
)

type credentials struct {
	Password   string `json:"password,omitempty"`
	Key        string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

func (h *Handler) pin(ctx context.Context, host string) (string, error) {
	ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if e != nil || len(ips) == 0 {
		return "", errors.New("接続先の名前解決に失敗しました")
	}
	for _, ip := range ips {
		ip = ip.Unmap()
		if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip == netip.MustParseAddr("fd00:ec2::254") || (!h.config.DevHTTP && ip.IsLoopback()) {
			return "", errors.New("接続先のアドレスは許可されていません")
		}
		allowed := false
		for _, cidr := range h.config.AllowedCIDRs {
			p, e := netip.ParsePrefix(cidr)
			if e == nil && p.Contains(ip) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", errors.New("接続先のアドレスは SSH_ALLOWED_CIDRS に含まれていません")
		}
	}
	return ips[0].Unmap().String(), nil
}
func (h *Handler) buildAuth(ctx context.Context, a control.Account, c credentials) (sshconn.ConnectRequest, error) {
	req := sshconn.ConnectRequest{User: a.Username, AuthType: a.AuthType}
	switch a.AuthType {
	case "password":
		if c.Key != "" || c.Passphrase != "" {
			return req, errors.New("credentials do not match password auth")
		}
		if c.Password == "" {
			return req, errors.New("SSH パスワードを入力してください")
		}
		req.Password = c.Password
	case "pubkey":
		if c.Password != "" {
			return req, errors.New("credentials do not match public-key auth")
		}
		if c.Key == "" {
			return req, errors.New("SSH 秘密鍵を指定してください")
		}
		req.UserPrivateKey = []byte(c.Key)
		req.UserPrivateKeyPassphrase = []byte(c.Passphrase)
	case "vault":
		if c.Password != "" || c.Key != "" || c.Passphrase != "" {
			return req, errors.New("Vault auth does not accept SSH credentials")
		}
		priv, pub, e := sshconn.GenerateKeyPair()
		if e != nil {
			return req, e
		}
		cert, e := h.vault.SignPublicKey(ctx, pub, a.Username)
		if e != nil {
			clear(priv)
			return req, errors.New("Vault certificate signing failed")
		}
		req.PrivateKey = priv
		req.Certificate = []byte(cert)
	default:
		return req, errors.New("invalid auth type")
	}
	return req, nil
}
func (h *Handler) connectRegistered(w http.ResponseWriter, r *http.Request) {
	i := who(r)
	var req struct {
		AccountID       string      `json:"target_account_id"`
		Credentials     credentials `json:"credentials"`
		JumpCredentials credentials `json:"jump_credentials"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !h.beginChange(w, r) {
		return
	}
	a, t, e := h.account(i.User.ID, req.AccountID, false)
	if e != nil {
		h.access.Unlock()
		missing(w)
		return
	}
	var ja control.Account
	var jt control.Target
	if t.JumpAccountID != "" {
		ja, jt, e = h.jump(t)
		if e != nil || jt.JumpAccountID != "" {
			h.access.Unlock()
			missing(w)
			return
		}
	}
	if t.JumpAccountID == "" && (req.JumpCredentials.Password != "" || req.JumpCredentials.Key != "" || req.JumpCredentials.Passphrase != "") {
		h.access.Unlock()
		invalid(w, errors.New("no jump is configured"))
		return
	}
	all := h.sessions.List()
	own := 0
	for _, s := range all {
		if s.OwnerUserID == i.User.ID {
			own++
		}
	}
	if own >= 32 || len(all)+len(h.pending) >= 256 {
		h.access.Unlock()
		apiError(w, 429, "接続数の上限に達しました", "SESSION_LIMIT")
		return
	}
	id := control.Random()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	h.pending[id] = cancel
	h.access.Unlock()
	defer cancel()
	defer func() { h.access.Lock(); delete(h.pending, id); h.access.Unlock() }()
	// All endpoint values come from registration. The checked IP is passed to the
	// transport directly; SSH host-key verification still uses the original host.
	ip, e := h.pin(ctx, t.Host)
	if e != nil {
		invalid(w, e)
		return
	}
	dialReq, e := h.buildAuth(ctx, a, req.Credentials)
	defer dialReq.ClearSecrets()
	if e != nil {
		apiError(w, 502, e.Error(), "AUTH_SETUP_ERROR")
		return
	}
	dialReq.Host, dialReq.Port, dialReq.DialIP = t.Host, t.Port, ip
	if t.JumpAccountID != "" {
		jip, e := h.pin(ctx, jt.Host)
		if e != nil {
			invalid(w, e)
			return
		}
		jump, e := h.buildAuth(ctx, ja, req.JumpCredentials)
		defer jump.ClearSecrets()
		if e != nil {
			apiError(w, 502, e.Error(), "AUTH_SETUP_ERROR")
			return
		}
		dialReq.JumpHost, dialReq.JumpPort, dialReq.JumpDialIP = jt.Host, jt.Port, jip
		dialReq.JumpUser, dialReq.JumpAuthType = ja.Username, ja.AuthType
		dialReq.JumpPassword = jump.Password
		dialReq.JumpPrivateKey, dialReq.JumpCertificate = jump.PrivateKey, jump.Certificate
		dialReq.JumpUserPrivateKey, dialReq.JumpUserPrivateKeyPassphrase = jump.UserPrivateKey, jump.UserPrivateKeyPassphrase
	}
	client, sshSess, stdin, stdout, e := h.dialer.Dial(ctx, dialReq)
	now := time.Now().Unix()
	l := control.Log{ID: id, Owner: i.User.ID, AccountID: a.ID, SessionID: id, Host: t.Host, Port: t.Port, Username: a.Username, Started: now}
	if e != nil {
		l.Ended = &now
		l.Error = classifyDialError(e)
		l.Reason = "ssh_dial_failed"
		if err := h.control.AddLog(l); err != nil {
			unavailable(w, err)
			return
		}
		apiError(w, 502, l.Error, "SSH_DIAL_ERROR")
		return
	}
	sess := session.NewSession(id, t.Host, t.Port, a.Username, client, sshSess, stdin, stdout, h.config.GracePeriod)
	sess.LogID = id
	sess.OwnerUserID = i.User.ID
	sess.TargetAccountID = a.ID
	h.access.Lock()
	defer h.access.Unlock()
	_, u, e := h.control.Login(i.Login.Hash)
	a2, t2, e2 := h.account(i.User.ID, a.ID, false)
	_, stillPending := h.pending[id]
	if e != nil || u.Version != i.User.Version || e2 != nil || a2 != a || t2 != t || !stillPending || ctx.Err() != nil {
		sess.Close()
		apiError(w, 409, "接続中に権限または設定が変更されました", "ACCESS_CHANGED")
		return
	}
	if t.JumpAccountID != "" {
		ja2, jt2, e := h.jump(t)
		if e != nil || ja2 != ja || jt2 != jt {
			sess.Close()
			apiError(w, 409, "踏み台の設定が変更されました", "ACCESS_CHANGED")
			return
		}
	}
	policy := h.policy()
	sess.ConfigureLimits(time.Duration(policy.GraceMinutes)*time.Minute, time.Duration(policy.SSHIdleMinutes)*time.Minute)
	if policy.Recording && a.Recording {
		if e = os.MkdirAll(h.config.RecordingDir, 0700); e != nil {
			sess.Close()
			unavailable(w, e)
			return
		}
		l.Path = filepath.Join(h.config.RecordingDir, id+".cast")
		rec, e := recording.New(l.Path, 80, 24, fmt.Sprintf("%s@%s", a.Username, t.Name))
		if e != nil {
			sess.Close()
			unavailable(w, e)
			return
		}
		sess.Recorder = rec
	}
	if e = h.control.AddLog(l); e != nil {
		sess.Close()
		if l.Path != "" {
			_ = os.Remove(l.Path)
		}
		unavailable(w, e)
		return
	}
	sess.OnClose = func(err error) {
		defer h.sessions.ForgetTerminated(id)
		reason := sess.Info().EndReason
		if e := h.control.EndLog(id, reason); e != nil {
			slog.Error("session log update failed", "id", id, "error", e)
		}
	}
	if e = h.sessions.Create(sess); e != nil {
		sess.Close()
		unavailable(w, e)
		return
	}
	writeJSON(w, 201, sess.Info())
}
func (h *Handler) appRecording(w http.ResponseWriter, r *http.Request) {
	owner := who(r).User.ID
	if adminPath(r) {
		owner = ""
	}
	l, e := h.control.Log(r.PathValue("id"), owner)
	if e != nil || l.Path == "" {
		missing(w)
		return
	}
	base, e := filepath.EvalSymlinks(h.config.RecordingDir)
	if e != nil {
		missing(w)
		return
	}
	base, e = filepath.Abs(base)
	if e != nil {
		unavailable(w, e)
		return
	}
	path, e := filepath.Abs(l.Path)
	if e != nil {
		missing(w)
		return
	}
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil || !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		missing(w)
		return
	}
	if adminPath(r) {
		if e = h.control.Mutation(who(r).User.ID, "recording.read", l.ID, "", func(_ *sql.Tx) error { return nil }); e != nil {
			unavailable(w, e)
			return
		}
	}
	w.Header().Set("Content-Type", "application/x-asciicast")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, resolved)
}
