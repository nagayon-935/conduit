package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the resolved configuration for conduit-cli.
type Config struct {
	VaultAddr     string
	VaultToken    string
	VaultSSHMount string
	VaultSSHRole  string
	DefaultAuth   string
	KnownHosts    string
	DefaultPort   int
	LogLevel      string
}

const (
	// DefaultVaultSSHMount is the default Vault SSH secrets engine mount path.
	DefaultVaultSSHMount = "ssh"
	// DefaultAuth is the default SSH authentication type.
	DefaultAuth = "vault"
	// DefaultPort is the default SSH port.
	DefaultPort = 22
	// DefaultLogLevel is the default log verbosity.
	DefaultLogLevel = "silent"
)

// RegisterEnvBindings binds environment variables to viper keys.
// This should be called before LoadConfig.
func RegisterEnvBindings() {
	_ = viper.BindEnv("vault.addr", "VAULT_ADDR")
	_ = viper.BindEnv("vault.token", "VAULT_TOKEN")
	_ = viper.BindEnv("vault.ssh_mount", "VAULT_SSH_MOUNT")
	_ = viper.BindEnv("vault.ssh_role", "VAULT_SSH_ROLE")
	_ = viper.BindEnv("ssh.default_auth", "CONDUIT_DEFAULT_AUTH")
	_ = viper.BindEnv("ssh.known_hosts", "CONDUIT_KNOWN_HOSTS")
	_ = viper.BindEnv("ssh.port", "CONDUIT_SSH_PORT")
	_ = viper.BindEnv("log.level", "CONDUIT_LOG_LEVEL")
}

// LoadConfig reads the configuration file (if present) and environment variables,
// applies defaults, and returns a resolved Config.
func LoadConfig(configPath string) (*Config, error) {
	viper.SetDefault("vault.ssh_mount", DefaultVaultSSHMount)
	viper.SetDefault("ssh.default_auth", DefaultAuth)
	viper.SetDefault("ssh.port", DefaultPort)
	viper.SetDefault("log.level", DefaultLogLevel)

	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("determine user home directory: %w", err)
		}
	}
	viper.SetDefault("ssh.known_hosts", filepath.Join(home, ".ssh", "known_hosts"))

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		if v := os.Getenv("CONDUIT_CONFIG"); v != "" {
			viper.AddConfigPath(v)
		}
		if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
			viper.AddConfigPath(filepath.Join(v, "conduit"))
		}
		viper.AddConfigPath(filepath.Join(home, ".config", "conduit"))
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	return buildConfig()
}

func buildConfig() (*Config, error) {
	knownHosts := os.ExpandEnv(viper.GetString("ssh.known_hosts"))

	cfg := &Config{
		VaultAddr:     viper.GetString("vault.addr"),
		VaultToken:    viper.GetString("vault.token"),
		VaultSSHMount: viper.GetString("vault.ssh_mount"),
		VaultSSHRole:  viper.GetString("vault.ssh_role"),
		DefaultAuth:   viper.GetString("ssh.default_auth"),
		KnownHosts:    knownHosts,
		DefaultPort:   viper.GetInt("ssh.port"),
		LogLevel:      viper.GetString("log.level"),
	}

	if cfg.VaultAddr == "" {
		return nil, fmt.Errorf("VAULT_ADDR environment variable is required")
	}
	if cfg.VaultToken == "" {
		return nil, fmt.Errorf("VAULT_TOKEN environment variable is required")
	}
	if cfg.VaultSSHRole == "" {
		return nil, fmt.Errorf("VAULT_SSH_ROLE environment variable is required")
	}

	if cfg.DefaultAuth != "vault" && cfg.DefaultAuth != "password" && cfg.DefaultAuth != "pubkey" {
		return nil, fmt.Errorf("invalid default auth type %q; must be vault, password, or pubkey", cfg.DefaultAuth)
	}

	return cfg, nil
}
