package cli

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nagayon-935/conduit/internal/sshconn"
	"github.com/nagayon-935/conduit/internal/vault"
	"golang.org/x/crypto/ssh"
)

// SSHArgs holds the parsed SSH connection arguments.
type SSHArgs struct {
	User         string
	Host         string
	Port         int
	AuthType     string
	Password     string
	Identity     string
	Passphrase   string
	Jump         *JumpArgs
	SSHExtraArgs []string
}

// JumpArgs holds ProxyJump connection arguments.
type JumpArgs struct {
	User       string
	Host       string
	Port       int
	AuthType   string
	Password   string
	Identity   string
	Passphrase string
}

// Runner executes SSH via the native ssh command using Vault-signed certificates.
type Runner struct {
	cfg         *Config
	vaultClient vault.VaultClient
	sshPath     string
	logger      *slog.Logger
}

// NewRunner creates a new Runner.
func NewRunner(cfg *Config, vaultClient vault.VaultClient, logger *slog.Logger) (*Runner, error) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return nil, fmt.Errorf("ssh command not found in PATH: %w", err)
	}
	return &Runner{
		cfg:         cfg,
		vaultClient: vaultClient,
		sshPath:     sshPath,
		logger:      logger,
	}, nil
}

// Run prepares keys/certificates and runs the ssh command.
// It returns the ssh exit code, or 0 with an error for internal failures.
func (r *Runner) Run(ctx context.Context, args SSHArgs) (int, error) {
	tmpDir, err := os.MkdirTemp("", "conduit-cli-*")
	if err != nil {
		return 0, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() {
		r.logger.Debug("cleaning up temp dir", "path", tmpDir)
		_ = os.RemoveAll(tmpDir)
	}()

	configPath := filepath.Join(tmpDir, "config")
	configBuilder := &strings.Builder{}

	if args.Jump != nil {
		if err := r.writeHostBlock(ctx, configBuilder, tmpDir, "conduit-jump-0", args.Jump.User, args.Jump.Host, args.Jump.Port, args.Jump.AuthType, args.Jump.Identity, args.Jump.Passphrase, ""); err != nil {
			return 0, err
		}
	}

	jumpRef := ""
	if args.Jump != nil {
		jumpRef = "conduit-jump-0"
	}
	if err := r.writeHostBlock(ctx, configBuilder, tmpDir, "target", args.User, args.Host, args.Port, args.AuthType, args.Identity, args.Passphrase, jumpRef); err != nil {
		return 0, err
	}

	if err := os.WriteFile(configPath, []byte(configBuilder.String()), 0600); err != nil {
		return 0, fmt.Errorf("write ssh config: %w", err)
	}

	sshArgs := []string{"-F", configPath}
	if shouldAllocateTTY() {
		sshArgs = append(sshArgs, "-t")
		r.logger.Debug("allocating TTY")
	} else {
		sshArgs = append(sshArgs, "-T")
		r.logger.Debug("not allocating TTY")
	}
	sshArgs = append(sshArgs, "target")
	sshArgs = append(sshArgs, args.SSHExtraArgs...)

	r.logger.Info("starting ssh", "args", strings.Join(sshArgs, " "))

	cmd := exec.CommandContext(ctx, r.sshPath, sshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start ssh: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 0, fmt.Errorf("wait ssh: %w", err)
	}

	return 0, nil
}

// writeHostBlock writes a Host block into the ssh_config builder.
func (r *Runner) writeHostBlock(ctx context.Context, b *strings.Builder, tmpDir, alias, user, host string, port int, authType, identity, passphrase, proxyJump string) error {
	fmt.Fprintf(b, "Host %s\n", alias)
	fmt.Fprintf(b, "    HostName %s\n", host)
	fmt.Fprintf(b, "    Port %d\n", port)
	fmt.Fprintf(b, "    User %s\n", user)

	if proxyJump != "" {
		fmt.Fprintf(b, "    ProxyJump %s\n", proxyJump)
	}

	switch authType {
	case "vault", "":
		privPEM, pubOpenSSH, err := sshconn.GenerateKeyPair()
		if err != nil {
			return fmt.Errorf("generate key pair: %w", err)
		}
		r.logger.Debug("signing certificate via vault", "alias", alias, "principal", user)
		cert, err := r.vaultClient.SignPublicKey(ctx, pubOpenSSH, user)
		if err != nil {
			return fmt.Errorf("vault signing failed: %w", err)
		}
		keyPath := filepath.Join(tmpDir, alias+"_key")
		certPath := filepath.Join(tmpDir, alias+"_cert")
		if err := os.WriteFile(keyPath, privPEM, 0600); err != nil {
			return fmt.Errorf("write private key: %w", err)
		}
		if err := os.WriteFile(certPath, []byte(cert), 0600); err != nil {
			return fmt.Errorf("write certificate: %w", err)
		}
		fmt.Fprintf(b, "    IdentityFile %s\n", keyPath)
		fmt.Fprintf(b, "    CertificateFile %s\n", certPath)

	case "pubkey":
		keyData, err := os.ReadFile(identity)
		if err != nil {
			return fmt.Errorf("read identity file %q: %w", identity, err)
		}
		// If a passphrase is provided, decrypt the key so ssh does not need to prompt.
		if passphrase != "" {
			keyData, err = decryptPrivateKey(keyData, []byte(passphrase))
			if err != nil {
				return fmt.Errorf("decrypt identity file: %w", err)
			}
		}
		keyPath := filepath.Join(tmpDir, alias+"_key")
		if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
			return fmt.Errorf("write identity key: %w", err)
		}
		fmt.Fprintf(b, "    IdentityFile %s\n", keyPath)

	case "password":
		// No identity file; the native ssh command prompts for the password.
	}

	fmt.Fprintf(b, "    UserKnownHostsFile %s\n", r.cfg.KnownHosts)
	b.WriteString("\n")
	return nil
}

// decryptPrivateKey decrypts an SSH private key and re-encodes it as PEM.
func decryptPrivateKey(keyData, passphrase []byte) ([]byte, error) {
	var rawKey interface{}
	var err error
	if len(passphrase) > 0 {
		rawKey, err = ssh.ParseRawPrivateKeyWithPassphrase(keyData, passphrase)
	} else {
		rawKey, err = ssh.ParseRawPrivateKey(keyData)
	}
	if err != nil {
		return nil, err
	}

	pemBlock, err := ssh.MarshalPrivateKey(rawKey, "")
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(pemBlock), nil
}
