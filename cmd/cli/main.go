package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nagayon-935/conduit/internal/cli"
	"github.com/nagayon-935/conduit/internal/vault"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	vaultHTTPTimeout = 10 * time.Second

	exitConfigError = 1
	exitVaultError  = 2
	exitKeyError    = 3
	exitSSHNotFound = 4
	exitInterrupted = 130
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		printError(err, exitConfigError)
	}
}

var rootCmd = &cobra.Command{
	Use:           "conduit-cli",
	Short:         "Conduit SSH client with Vault certificate authentication",
	Long:          `conduit-cli is a command-line SSH client that uses HashiCorp Vault's
SSH Secrets Engine to sign short-lived certificates for authentication.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var sshCmd = &cobra.Command{
	Use:           "ssh [flags] [user@]host[:port] [-- [remote-command...]]",
	Short:         "Connect to a host via SSH",
	Args:          cobra.MinimumNArgs(1),
	RunE:          runSSH,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(os.Stdout, "conduit-cli version 0.1.0")
	},
}

func init() {
	rootCmd.PersistentFlags().String("config", "", "Path to config file")
	rootCmd.PersistentFlags().String("vault-addr", "", "Vault address (overrides VAULT_ADDR)")
	rootCmd.PersistentFlags().String("vault-ssh-mount", "", "Vault SSH mount path (overrides VAULT_SSH_MOUNT)")
	rootCmd.PersistentFlags().String("vault-ssh-role", "", "Vault SSH role name (overrides VAULT_SSH_ROLE)")
	rootCmd.PersistentFlags().String("known-hosts", "", "Path to known_hosts file")
	rootCmd.PersistentFlags().String("log-level", "", "Log level: silent, info, debug")

	sshCmd.Flags().IntP("port", "p", 0, "SSH port")
	sshCmd.Flags().StringP("auth", "A", "", "Authentication type: vault, password, pubkey")
	sshCmd.Flags().StringP("identity", "i", "", "Path to private key file (for pubkey auth)")
	sshCmd.Flags().String("passphrase", "", "Passphrase for the private key file")
	sshCmd.Flags().StringP("jump", "J", "", "ProxyJump host as [user@]host[:port]")
	sshCmd.Flags().String("jump-auth", "", "ProxyJump authentication type: vault, password, pubkey")
	sshCmd.Flags().String("jump-identity", "", "Path to ProxyJump private key file")
	sshCmd.Flags().String("jump-passphrase", "", "Passphrase for the ProxyJump private key")
	sshCmd.Flags().CountP("verbose", "v", "Increase verbosity (-v=info, -vv=debug)")

	rootCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(versionCmd)

	cli.RegisterEnvBindings()

	bindPFlag(rootCmd, "vault.addr", "vault-addr")
	bindPFlag(rootCmd, "vault.ssh_mount", "vault-ssh-mount")
	bindPFlag(rootCmd, "vault.ssh_role", "vault-ssh-role")
	bindPFlag(rootCmd, "ssh.known_hosts", "known-hosts")
	bindPFlag(rootCmd, "log.level", "log-level")
	bindPFlag(sshCmd, "ssh.port", "port")
	bindPFlag(sshCmd, "ssh.default_auth", "auth")
}

func runSSH(cmd *cobra.Command, args []string) error {
	configPath := getStringFlag(rootCmd, "config")
	cfg, err := cli.LoadConfig(configPath)
	if err != nil {
		return &exitError{code: exitConfigError, err: err}
	}

	logLevel := cfg.LogLevel
	if verbose, _ := cmd.Flags().GetCount("verbose"); verbose > 0 {
		if verbose >= 2 {
			logLevel = "debug"
		} else {
			logLevel = "info"
		}
	}
	logger := newLogger(logLevel)

	targetUser, targetHost, targetPort, err := parseTarget(args[0], cfg.DefaultPort)
	if err != nil {
		return &exitError{code: exitConfigError, err: err}
	}

	authType := getStringFlag(cmd, "auth")
	if authType == "" {
		authType = cfg.DefaultAuth
	}
	if err := validateAuth(authType); err != nil {
		return &exitError{code: exitConfigError, err: err}
	}

	sshArgs := cli.SSHArgs{
		User:         targetUser,
		Host:         targetHost,
		Port:         targetPort,
		AuthType:     authType,
		Identity:     getStringFlag(cmd, "identity"),
		Passphrase:   getStringFlag(cmd, "passphrase"),
		SSHExtraArgs: args[1:],
	}

	if authType == "pubkey" && sshArgs.Identity == "" {
		return &exitError{code: exitConfigError, err: errors.New("--identity is required for pubkey auth")}
	}

	if jumpStr := getStringFlag(cmd, "jump"); jumpStr != "" {
		jumpUser, jumpHost, jumpPort, err := parseTarget(jumpStr, cfg.DefaultPort)
		if err != nil {
			return &exitError{code: exitConfigError, err: fmt.Errorf("parse jump host: %w", err)}
		}
		jumpAuth := getStringFlag(cmd, "jump-auth")
		if jumpAuth == "" {
			jumpAuth = cfg.DefaultAuth
		}
		if err := validateAuth(jumpAuth); err != nil {
			return &exitError{code: exitConfigError, err: err}
		}
		if jumpAuth == "pubkey" && getStringFlag(cmd, "jump-identity") == "" {
			return &exitError{code: exitConfigError, err: errors.New("--jump-identity is required for jump host pubkey auth")}
		}
		sshArgs.Jump = &cli.JumpArgs{
			User:       jumpUser,
			Host:       jumpHost,
			Port:       jumpPort,
			AuthType:   jumpAuth,
			Identity:   getStringFlag(cmd, "jump-identity"),
			Passphrase: getStringFlag(cmd, "jump-passphrase"),
		}
	}

	vaultClient, err := vault.NewClientWithTimeout(cfg.VaultAddr, cfg.VaultToken, cfg.VaultSSHMount, cfg.VaultSSHRole, vaultHTTPTimeout)
	if err != nil {
		return &exitError{code: exitConfigError, err: fmt.Errorf("create vault client: %w", err)}
	}

	runner, err := cli.NewRunner(cfg, vaultClient, logger)
	if err != nil {
		return &exitError{code: exitSSHNotFound, err: err}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	code, err := runner.Run(ctx, sshArgs)
	if err != nil {
		return classifyError(err)
	}
	os.Exit(code)
	return nil
}

func bindPFlag(cmd *cobra.Command, key, flagName string) {
	flagSet := cmd.Flags()
	if cmd.PersistentFlags().Lookup(flagName) != nil {
		flagSet = cmd.PersistentFlags()
	}
	if err := viper.BindPFlag(key, flagSet.Lookup(flagName)); err != nil {
		panic(fmt.Sprintf("bind flag %q: %v", flagName, err))
	}
}

func getStringFlag(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

func validateAuth(auth string) error {
	switch auth {
	case "vault", "password", "pubkey":
		return nil
	default:
		return fmt.Errorf("invalid auth type %q; must be vault, password, or pubkey", auth)
	}
}

// parseTarget parses [user@]host[:port].
// If user is omitted, the current OS user is used.
func parseTarget(s string, defaultPort int) (user, host string, port int, err error) {
	port = defaultPort

	if at := strings.LastIndex(s, "@"); at >= 0 {
		user = s[:at]
		s = s[at+1:]
	}
	if user == "" {
		u, err := currentUsername()
		if err != nil {
			return "", "", 0, fmt.Errorf("determine current user: %w", err)
		}
		user = u
	}

	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		host = s
	} else {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return "", "", 0, fmt.Errorf("invalid port %q", portStr)
		}
		port = p
	}

	if host == "" {
		return "", "", 0, errors.New("host is required")
	}

	return user, host, port, nil
}

func currentUsername() (string, error) {
	if u := os.Getenv("USER"); u != "" {
		return u, nil
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u, nil
	}
	return "", errors.New("USER or USERNAME environment variable is not set")
}

func newLogger(level string) *slog.Logger {
	var programLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		programLevel = slog.LevelDebug
	case "info":
		programLevel = slog.LevelInfo
	case "warn":
		programLevel = slog.LevelWarn
	case "error":
		programLevel = slog.LevelError
	default:
		programLevel = slog.LevelError + 1 // silent
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel})
	return slog.New(handler)
}

// exitError wraps an error with a specific exit code.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func printError(err error, defaultCode int) {
	var ee *exitError
	code := defaultCode
	if errors.As(err, &ee) {
		code = ee.code
		err = ee.err
	}
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(code)
}

func classifyError(err error) error {
	if errors.Is(err, context.Canceled) {
		return &exitError{code: exitInterrupted, err: errors.New("interrupted")}
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "vault signing failed"):
		return &exitError{code: exitVaultError, err: err}
	case strings.Contains(msg, "generate key pair"):
		return &exitError{code: exitKeyError, err: err}
	case strings.Contains(msg, "ssh command not found"):
		return &exitError{code: exitSSHNotFound, err: err}
	default:
		return &exitError{code: exitConfigError, err: err}
	}
}
