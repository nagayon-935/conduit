package cli

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
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
