package config_test

// NOTE: t.Setenv は並列テストで使用できないため、このファイルでは t.Parallel() を呼ばない。
// t.Setenv は Cleanup で自動的に元の値に戻すため、テスト間の干渉は起きない。

import (
	"strings"
	"testing"
	"time"

	"github.com/nagayon-935/conduit/internal/config"
)

// setAllRequired は3つの必須環境変数をまとめてセットするヘルパー。
func setAllRequired(t *testing.T, vaultAddr, vaultToken, sshRole string) {
	t.Helper()
	t.Setenv("VAULT_ADDR", vaultAddr)
	t.Setenv("VAULT_TOKEN", vaultToken)
	t.Setenv("VAULT_SSH_ROLE", sshRole)
}

// TestLoad_AllFields は全フィールドが環境変数から正しく読み込まれることを検証する。
func TestLoad_AllFields(t *testing.T) {
	setAllRequired(t, "http://vault.test:8200", "s.mytoken", "conduit-role")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.VaultAddr != "http://vault.test:8200" {
		t.Errorf("VaultAddr = %q, want %q", cfg.VaultAddr, "http://vault.test:8200")
	}
	if cfg.VaultToken.Value() != "s.mytoken" {
		t.Errorf("VaultToken = %q, want %q", cfg.VaultToken.Value(), "s.mytoken")
	}
	if cfg.VaultSSHRole != "conduit-role" {
		t.Errorf("VaultSSHRole = %q, want %q", cfg.VaultSSHRole, "conduit-role")
	}
}

// TestLoad_Defaults はオプション項目のデフォルト値が正しく設定されることを検証する。
func TestLoad_Defaults(t *testing.T) {
	setAllRequired(t, "http://vault.test:8200", "tok", "role")
	// オプション項目はセットしない → デフォルト値が使われるはず
	t.Setenv("SERVER_PORT", "")
	t.Setenv("VAULT_SSH_MOUNT", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ServerAddr != ":8080" {
		t.Errorf("ServerAddr = %q, want %q", cfg.ServerAddr, ":8080")
	}
	if cfg.VaultSSHMount != "ssh" {
		t.Errorf("VaultSSHMount = %q, want %q", cfg.VaultSSHMount, "ssh")
	}
	if cfg.GracePeriod != 15*time.Minute {
		t.Errorf("GracePeriod = %v, want %v", cfg.GracePeriod, 15*time.Minute)
	}
	if cfg.SessionGCInterval != 1*time.Minute {
		t.Errorf("SessionGCInterval = %v, want %v", cfg.SessionGCInterval, time.Minute)
	}
}

// TestLoad_OverrideDefaults はオプション項目を明示的に上書きできることを検証する。
func TestLoad_OverrideDefaults(t *testing.T) {
	setAllRequired(t, "http://vault.test:8200", "tok", "role")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("VAULT_SSH_MOUNT", "custom-ssh")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ServerAddr != ":9090" {
		t.Errorf("ServerAddr = %q, want %q", cfg.ServerAddr, ":9090")
	}
	if cfg.VaultSSHMount != "custom-ssh" {
		t.Errorf("VaultSSHMount = %q, want %q", cfg.VaultSSHMount, "custom-ssh")
	}
}

// TestLoad_MissingVaultAddr は VAULT_ADDR が未設定の場合にエラーを返すことを検証する。
func TestLoad_MissingVaultAddr(t *testing.T) {
	t.Setenv("VAULT_ADDR", "") // 未設定を模倣
	t.Setenv("VAULT_TOKEN", "tok")
	t.Setenv("VAULT_SSH_ROLE", "role")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing VAULT_ADDR, got nil")
	}
	if !strings.Contains(err.Error(), "VAULT_ADDR") {
		t.Errorf("error message %q should mention VAULT_ADDR", err.Error())
	}
}

// TestLoad_MissingVaultToken は VAULT_TOKEN が未設定の場合にエラーを返すことを検証する。
func TestLoad_MissingVaultToken(t *testing.T) {
	t.Setenv("VAULT_ADDR", "http://vault.test:8200")
	t.Setenv("VAULT_TOKEN", "") // 未設定を模倣
	t.Setenv("VAULT_SSH_ROLE", "role")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing VAULT_TOKEN, got nil")
	}
	if !strings.Contains(err.Error(), "VAULT_TOKEN") {
		t.Errorf("error message %q should mention VAULT_TOKEN", err.Error())
	}
}

// TestLoad_MissingVaultSSHRole は VAULT_SSH_ROLE が未設定の場合にエラーを返すことを検証する。
func TestLoad_MissingVaultSSHRole(t *testing.T) {
	t.Setenv("VAULT_ADDR", "http://vault.test:8200")
	t.Setenv("VAULT_TOKEN", "tok")
	t.Setenv("VAULT_SSH_ROLE", "") // 未設定を模倣

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing VAULT_SSH_ROLE, got nil")
	}
	if !strings.Contains(err.Error(), "VAULT_SSH_ROLE") {
		t.Errorf("error message %q should mention VAULT_SSH_ROLE", err.Error())
	}
}

// ── IdleTimeout ──────────────────────────────────────────────────────────────

// TestLoad_IdleTimeout_Default verifies the 30-minute default when
// SESSION_IDLE_TIMEOUT is not set.
func TestLoad_IdleTimeout_Default(t *testing.T) {
	setAllRequired(t, "http://vault.test:8200", "tok", "role")
	t.Setenv("SESSION_IDLE_TIMEOUT", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.IdleTimeout != 30*time.Minute {
		t.Errorf("IdleTimeout = %v, want %v", cfg.IdleTimeout, 30*time.Minute)
	}
}

// TestLoad_IdleTimeout_Override verifies a custom duration is honored.
func TestLoad_IdleTimeout_Override(t *testing.T) {
	setAllRequired(t, "http://vault.test:8200", "tok", "role")
	t.Setenv("SESSION_IDLE_TIMEOUT", "45m")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.IdleTimeout != 45*time.Minute {
		t.Errorf("IdleTimeout = %v, want %v", cfg.IdleTimeout, 45*time.Minute)
	}
}

// TestLoad_IdleTimeout_ZeroDisables verifies "0" explicitly disables the idle timeout.
func TestLoad_IdleTimeout_ZeroDisables(t *testing.T) {
	setAllRequired(t, "http://vault.test:8200", "tok", "role")
	t.Setenv("SESSION_IDLE_TIMEOUT", "0")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.IdleTimeout != 0 {
		t.Errorf("IdleTimeout = %v, want 0", cfg.IdleTimeout)
	}
}

// TestLoad_IdleTimeout_Invalid verifies an unparseable duration is rejected.
func TestLoad_IdleTimeout_Invalid(t *testing.T) {
	setAllRequired(t, "http://vault.test:8200", "tok", "role")
	t.Setenv("SESSION_IDLE_TIMEOUT", "not-a-duration")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid SESSION_IDLE_TIMEOUT, got nil")
	}
	if !strings.Contains(err.Error(), "SESSION_IDLE_TIMEOUT") {
		t.Errorf("error message %q should mention SESSION_IDLE_TIMEOUT", err.Error())
	}
}

// TestLoad_IdleTimeout_Negative verifies a negative duration is rejected.
func TestLoad_IdleTimeout_Negative(t *testing.T) {
	setAllRequired(t, "http://vault.test:8200", "tok", "role")
	t.Setenv("SESSION_IDLE_TIMEOUT", "-5m")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for negative SESSION_IDLE_TIMEOUT, got nil")
	}
}

// ── AdminAPIToken ────────────────────────────────────────────────────────────

// TestLoad_AdminAPIToken_DefaultEmpty verifies admin endpoints stay open (empty
// token) when ADMIN_API_TOKEN is not set, preserving lab-default behavior.
func TestLoad_AdminAPIToken_DefaultEmpty(t *testing.T) {
	setAllRequired(t, "http://vault.test:8200", "tok", "role")
	t.Setenv("ADMIN_API_TOKEN", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AdminAPIToken.Value() != "" {
		t.Errorf("AdminAPIToken = %q, want empty", cfg.AdminAPIToken.Value())
	}
}

// TestLoad_AdminAPIToken_Set verifies ADMIN_API_TOKEN is read into config.
func TestLoad_AdminAPIToken_Set(t *testing.T) {
	setAllRequired(t, "http://vault.test:8200", "tok", "role")
	t.Setenv("ADMIN_API_TOKEN", "super-secret-admin-token")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AdminAPIToken.Value() != "super-secret-admin-token" {
		t.Errorf("AdminAPIToken = %q, want %q", cfg.AdminAPIToken.Value(), "super-secret-admin-token")
	}
}
