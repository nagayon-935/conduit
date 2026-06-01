package api

import "testing"

// TestValidateConnectRequest exercises every branch of validateConnectRequest,
// including the ProxyJump validation paths not reached by the handler tests.
func TestValidateConnectRequest(t *testing.T) {
	t.Parallel()

	base := func() ConnectRequest {
		return ConnectRequest{Host: "10.0.0.1", Port: 22, User: "ubuntu", AuthType: "vault"}
	}

	tests := []struct {
		name    string
		mutate  func(*ConnectRequest)
		wantErr bool
	}{
		// ── Valid ────────────────────────────────────────────────────────────
		{"vault minimal", func(r *ConnectRequest) {}, false},
		{"password with password", func(r *ConnectRequest) { r.AuthType = "password"; r.Password = "pw" }, false},
		{"pubkey with key", func(r *ConnectRequest) { r.AuthType = "pubkey"; r.PrivateKey = "KEY" }, false},
		{"jump vault valid", func(r *ConnectRequest) { r.JumpHost = "bastion"; r.JumpUser = "ubuntu"; r.JumpAuthType = "vault" }, false},
		{"jump password valid", func(r *ConnectRequest) {
			r.JumpHost = "bastion"
			r.JumpUser = "ubuntu"
			r.JumpAuthType = "password"
			r.JumpPassword = "pw"
		}, false},
		{"jump pubkey valid", func(r *ConnectRequest) {
			r.JumpHost = "bastion"
			r.JumpUser = "ubuntu"
			r.JumpAuthType = "pubkey"
			r.JumpPrivateKey = "KEY"
		}, false},
		{"jump port zero allowed (defaulted later)", func(r *ConnectRequest) {
			r.JumpHost = "bastion"
			r.JumpUser = "ubuntu"
			r.JumpPort = 0
		}, false},

		// ── Invalid: primary host ────────────────────────────────────────────
		{"empty host", func(r *ConnectRequest) { r.Host = "" }, true},
		{"whitespace host", func(r *ConnectRequest) { r.Host = "   " }, true},
		{"port zero", func(r *ConnectRequest) { r.Port = 0 }, true},
		{"port too large", func(r *ConnectRequest) { r.Port = 65536 }, true},
		{"empty user", func(r *ConnectRequest) { r.User = "" }, true},
		{"password auth without password", func(r *ConnectRequest) { r.AuthType = "password" }, true},
		{"pubkey auth without key", func(r *ConnectRequest) { r.AuthType = "pubkey" }, true},

		// ── Invalid: jump host ───────────────────────────────────────────────
		{"jump host without user", func(r *ConnectRequest) { r.JumpHost = "bastion" }, true},
		{"jump port too large", func(r *ConnectRequest) {
			r.JumpHost = "bastion"
			r.JumpUser = "ubuntu"
			r.JumpPort = 65536
		}, true},
		{"jump password auth without password", func(r *ConnectRequest) {
			r.JumpHost = "bastion"
			r.JumpUser = "ubuntu"
			r.JumpAuthType = "password"
		}, true},
		{"jump pubkey auth without key", func(r *ConnectRequest) {
			r.JumpHost = "bastion"
			r.JumpUser = "ubuntu"
			r.JumpAuthType = "pubkey"
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := base()
			tt.mutate(&req)
			err := validateConnectRequest(req)
			if tt.wantErr && err == nil {
				t.Errorf("validateConnectRequest() = nil; want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateConnectRequest() = %v; want nil", err)
			}
		})
	}
}
