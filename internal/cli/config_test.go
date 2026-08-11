package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func resetViper(t *testing.T) {
	t.Helper()
	viper.Reset()
	RegisterEnvBindings()
}

func TestLoadConfig_Defaults(t *testing.T) {
	resetViper(t)
	os.Setenv("VAULT_ADDR", "https://vault.example.com:8200")
	os.Setenv("VAULT_TOKEN", "test-token")
	os.Setenv("VAULT_SSH_ROLE", "conduit-cli-role")
	defer func() {
		os.Unsetenv("VAULT_ADDR")
		os.Unsetenv("VAULT_TOKEN")
		os.Unsetenv("VAULT_SSH_ROLE")
	}()

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: unexpected error: %v", err)
	}

	if cfg.VaultAddr != "https://vault.example.com:8200" {
		t.Errorf("VaultAddr = %q, want %q", cfg.VaultAddr, "https://vault.example.com:8200")
	}
	if cfg.VaultSSHMount != "ssh" {
		t.Errorf("VaultSSHMount = %q, want %q", cfg.VaultSSHMount, "ssh")
	}
	if cfg.DefaultAuth != "vault" {
		t.Errorf("DefaultAuth = %q, want %q", cfg.DefaultAuth, "vault")
	}
	if cfg.DefaultPort != 22 {
		t.Errorf("DefaultPort = %d, want %d", cfg.DefaultPort, 22)
	}
	if cfg.LogLevel != "silent" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "silent")
	}
	if cfg.KnownHosts == "" {
		t.Error("KnownHosts should have a default value")
	}
}

func TestLoadConfig_MissingVaultAddr(t *testing.T) {
	resetViper(t)
	os.Setenv("VAULT_TOKEN", "test-token")
	os.Setenv("VAULT_SSH_ROLE", "conduit-cli-role")
	defer func() {
		os.Unsetenv("VAULT_TOKEN")
		os.Unsetenv("VAULT_SSH_ROLE")
	}()

	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("LoadConfig: expected error for missing VAULT_ADDR")
	}
}

func TestLoadConfig_MissingVaultToken(t *testing.T) {
	resetViper(t)
	os.Setenv("VAULT_ADDR", "https://vault.example.com:8200")
	os.Setenv("VAULT_SSH_ROLE", "conduit-cli-role")
	defer func() {
		os.Unsetenv("VAULT_ADDR")
		os.Unsetenv("VAULT_SSH_ROLE")
	}()

	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("LoadConfig: expected error for missing VAULT_TOKEN")
	}
}

func TestLoadConfig_MissingVaultSSHRole(t *testing.T) {
	resetViper(t)
	os.Setenv("VAULT_ADDR", "https://vault.example.com:8200")
	os.Setenv("VAULT_TOKEN", "test-token")
	defer func() {
		os.Unsetenv("VAULT_ADDR")
		os.Unsetenv("VAULT_TOKEN")
	}()

	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("LoadConfig: expected error for missing VAULT_SSH_ROLE")
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	resetViper(t)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `
vault:
  addr: "https://vault.file.example.com:8200"
  ssh_mount: "ssh-engine"
  ssh_role: "cli-role"
ssh:
  default_auth: "pubkey"
  known_hosts: "$HOME/.ssh/known_hosts"
  port: 2222
log:
  level: "debug"
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	os.Setenv("VAULT_TOKEN", "test-token")
	defer os.Unsetenv("VAULT_TOKEN")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: unexpected error: %v", err)
	}

	if cfg.VaultAddr != "https://vault.file.example.com:8200" {
		t.Errorf("VaultAddr = %q, want %q", cfg.VaultAddr, "https://vault.file.example.com:8200")
	}
	if cfg.VaultSSHMount != "ssh-engine" {
		t.Errorf("VaultSSHMount = %q, want %q", cfg.VaultSSHMount, "ssh-engine")
	}
	if cfg.VaultSSHRole != "cli-role" {
		t.Errorf("VaultSSHRole = %q, want %q", cfg.VaultSSHRole, "cli-role")
	}
	if cfg.DefaultAuth != "pubkey" {
		t.Errorf("DefaultAuth = %q, want %q", cfg.DefaultAuth, "pubkey")
	}
	if cfg.DefaultPort != 2222 {
		t.Errorf("DefaultPort = %d, want %d", cfg.DefaultPort, 2222)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoadConfig_InvalidAuthType(t *testing.T) {
	resetViper(t)
	os.Setenv("VAULT_ADDR", "https://vault.example.com:8200")
	os.Setenv("VAULT_TOKEN", "test-token")
	os.Setenv("VAULT_SSH_ROLE", "conduit-cli-role")
	os.Setenv("CONDUIT_DEFAULT_AUTH", "invalid")
	defer func() {
		os.Unsetenv("VAULT_ADDR")
		os.Unsetenv("VAULT_TOKEN")
		os.Unsetenv("VAULT_SSH_ROLE")
		os.Unsetenv("CONDUIT_DEFAULT_AUTH")
	}()

	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("LoadConfig: expected error for invalid auth type")
	}
}
