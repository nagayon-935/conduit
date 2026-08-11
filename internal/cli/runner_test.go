package cli

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nagayon-935/conduit/internal/vault"
	"golang.org/x/crypto/ssh"
)

type mockVaultClient struct {
	signedKey string
	err       error
}

func (m *mockVaultClient) SignPublicKey(ctx context.Context, publicKey, principal string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.signedKey, nil
}

var _ vault.VaultClient = (*mockVaultClient)(nil)

func TestWriteHostBlock_VaultAuth(t *testing.T) {
	cfg := &Config{KnownHosts: "$HOME/.ssh/known_hosts"}
	r := &Runner{
		cfg:         cfg,
		vaultClient: &mockVaultClient{signedKey: "ssh-ed25519-cert-v1..."},
		logger:      newTestLogger(),
	}

	tmpDir := t.TempDir()
	b := &strings.Builder{}

	if err := r.writeHostBlock(context.Background(), b, tmpDir, "target", "alice", "host.example.com", 22, "vault", "", "", ""); err != nil {
		t.Fatalf("writeHostBlock: unexpected error: %v", err)
	}

	out := b.String()
	if !strings.Contains(out, "Host target") {
		t.Errorf("config missing Host target")
	}
	if !strings.Contains(out, "HostName host.example.com") {
		t.Errorf("config missing HostName")
	}
	if !strings.Contains(out, "User alice") {
		t.Errorf("config missing User")
	}
	if !strings.Contains(out, "IdentityFile") {
		t.Errorf("config missing IdentityFile")
	}
	if !strings.Contains(out, "CertificateFile") {
		t.Errorf("config missing CertificateFile")
	}
	if !strings.Contains(out, "UserKnownHostsFile") {
		t.Errorf("config missing UserKnownHostsFile")
	}
}

func TestWriteHostBlock_PubkeyAuth(t *testing.T) {
	cfg := &Config{KnownHosts: "$HOME/.ssh/known_hosts"}
	r := &Runner{
		cfg:    cfg,
		logger: newTestLogger(),
	}

	// Generate a test key file.
	tmpDir := t.TempDir()
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	pemBlock, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_ed25519")
	if err := os.WriteFile(identityPath, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		t.Fatalf("write identity file: %v", err)
	}

	workDir := t.TempDir()
	b := &strings.Builder{}

	if err := r.writeHostBlock(context.Background(), b, workDir, "target", "alice", "host.example.com", 22, "pubkey", identityPath, "", ""); err != nil {
		t.Fatalf("writeHostBlock: unexpected error: %v", err)
	}

	out := b.String()
	if !strings.Contains(out, "IdentityFile") {
		t.Errorf("config missing IdentityFile")
	}
	if strings.Contains(out, "CertificateFile") {
		t.Error("pubkey config should not contain CertificateFile")
	}
}

func TestWriteHostBlock_PasswordAuth(t *testing.T) {
	cfg := &Config{KnownHosts: "$HOME/.ssh/known_hosts"}
	r := &Runner{
		cfg:    cfg,
		logger: newTestLogger(),
	}

	tmpDir := t.TempDir()
	b := &strings.Builder{}

	if err := r.writeHostBlock(context.Background(), b, tmpDir, "target", "alice", "host.example.com", 22, "password", "", "", ""); err != nil {
		t.Fatalf("writeHostBlock: unexpected error: %v", err)
	}

	out := b.String()
	if strings.Contains(out, "IdentityFile") {
		t.Error("password config should not contain IdentityFile")
	}
	if strings.Contains(out, "CertificateFile") {
		t.Error("password config should not contain CertificateFile")
	}
}

func TestDecryptPrivateKey(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	passphrase := []byte("test-passphrase")
	pemBlock, err := ssh.MarshalPrivateKeyWithPassphrase(privKey, "", passphrase)
	if err != nil {
		t.Fatalf("marshal encrypted private key: %v", err)
	}
	keyData := pem.EncodeToMemory(pemBlock)

	decrypted, err := decryptPrivateKey(keyData, passphrase)
	if err != nil {
		t.Fatalf("decryptPrivateKey: unexpected error: %v", err)
	}

	if _, err := ssh.ParseRawPrivateKey(decrypted); err != nil {
		t.Errorf("parsed decrypted key: %v", err)
	}
}

func TestDecryptPrivateKey_WrongPassphrase(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	pemBlock, err := ssh.MarshalPrivateKeyWithPassphrase(privKey, "", []byte("correct"))
	if err != nil {
		t.Fatalf("marshal encrypted private key: %v", err)
	}
	keyData := pem.EncodeToMemory(pemBlock)

	_, err = decryptPrivateKey(keyData, []byte("wrong"))
	if err == nil {
		t.Fatal("decryptPrivateKey: expected error for wrong passphrase")
	}
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

func TestNewRunner_SSHNotFound(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", origPath)

	cfg := &Config{}
	_, err := NewRunner(cfg, &mockVaultClient{}, newTestLogger())
	if err == nil {
		t.Fatal("NewRunner: expected error when ssh is not found")
	}
}

func TestRunner_Run_VaultDirect(t *testing.T) {
	fakeSSH := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(fakeSSH)+":"+os.Getenv("PATH"))

	cfg := &Config{KnownHosts: filepath.Join(t.TempDir(), "known_hosts")}
	if err := os.WriteFile(cfg.KnownHosts, []byte{}, 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	r, err := NewRunner(cfg, &mockVaultClient{signedKey: "ssh-ed25519-cert-v1 AAAAC3NzaC1lZDI1NTE5..."}, newTestLogger())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	code, err := r.Run(context.Background(), SSHArgs{
		User:     "alice",
		Host:     "host.example.com",
		Port:     22,
		AuthType: "vault",
	})
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRunner_Run_VaultProxyJump(t *testing.T) {
	fakeSSH := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(fakeSSH)+":"+os.Getenv("PATH"))

	cfg := &Config{KnownHosts: filepath.Join(t.TempDir(), "known_hosts")}
	if err := os.WriteFile(cfg.KnownHosts, []byte{}, 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	r, err := NewRunner(cfg, &mockVaultClient{signedKey: "ssh-ed25519-cert-v1 AAAAC3NzaC1lZDI1NTE5..."}, newTestLogger())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	code, err := r.Run(context.Background(), SSHArgs{
		User:     "alice",
		Host:     "target.internal",
		Port:     22,
		AuthType: "vault",
		Jump: &JumpArgs{
			User:     "admin",
			Host:     "bastion.example.com",
			Port:     2222,
			AuthType: "vault",
		},
	})
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRunner_Run_SSHExitCode(t *testing.T) {
	fakeSSH := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 42\n"), 0755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(fakeSSH)+":"+os.Getenv("PATH"))

	cfg := &Config{KnownHosts: filepath.Join(t.TempDir(), "known_hosts")}
	if err := os.WriteFile(cfg.KnownHosts, []byte{}, 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	r, err := NewRunner(cfg, &mockVaultClient{signedKey: "ssh-ed25519-cert-v1 AAAAC3NzaC1lZDI1NTE5..."}, newTestLogger())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	code, err := r.Run(context.Background(), SSHArgs{
		User:     "alice",
		Host:     "host.example.com",
		Port:     22,
		AuthType: "vault",
	})
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if code != 42 {
		t.Errorf("exit code = %d, want 42", code)
	}
}

func TestRunner_Run_VaultSigningError(t *testing.T) {
	fakeSSH := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(fakeSSH)+":"+os.Getenv("PATH"))

	cfg := &Config{KnownHosts: filepath.Join(t.TempDir(), "known_hosts")}
	if err := os.WriteFile(cfg.KnownHosts, []byte{}, 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	r, err := NewRunner(cfg, &mockVaultClient{err: errors.New("permission denied")}, newTestLogger())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	_, err = r.Run(context.Background(), SSHArgs{
		User:     "alice",
		Host:     "host.example.com",
		Port:     22,
		AuthType: "vault",
	})
	if err == nil {
		t.Fatal("Run: expected vault signing error")
	}
}

func TestRunner_Run_PubkeyWithPassphrase(t *testing.T) {
	fakeSSH := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(fakeSSH)+":"+os.Getenv("PATH"))

	cfg := &Config{KnownHosts: filepath.Join(t.TempDir(), "known_hosts")}
	if err := os.WriteFile(cfg.KnownHosts, []byte{}, 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	// Generate encrypted key.
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	pemBlock, err := ssh.MarshalPrivateKeyWithPassphrase(privKey, "", []byte("secret"))
	if err != nil {
		t.Fatalf("marshal encrypted key: %v", err)
	}
	identityPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(identityPath, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		t.Fatalf("write identity file: %v", err)
	}

	r, err := NewRunner(cfg, &mockVaultClient{}, newTestLogger())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	code, err := r.Run(context.Background(), SSHArgs{
		User:       "alice",
		Host:       "host.example.com",
		Port:       22,
		AuthType:   "pubkey",
		Identity:   identityPath,
		Passphrase: "secret",
	})
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}
