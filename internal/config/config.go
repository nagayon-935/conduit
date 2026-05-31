package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all application configuration values.
type Config struct {
	ServerAddr        string
	VaultAddr         string
	VaultToken        Secret
	VaultSSHMount     string
	VaultSSHRole      string
	GracePeriod       time.Duration
	SessionGCInterval time.Duration
	// AllowedOrigins is the list of CORS origins permitted to access the API.
	// Loaded from CORS_ALLOWED_ORIGINS (comma-separated). Defaults to localhost:5173.
	AllowedOrigins []string
	// KnownHostsPath is the path to the SSH known_hosts file used for host key verification.
	// Loaded from KNOWN_HOSTS_PATH.
	KnownHostsPath string
	// DBPath is the path to the SQLite database file for persistent logs.
	// Loaded from DB_PATH. When empty, an in-memory log store is used.
	DBPath string
}

// Load reads configuration from environment variables and applies defaults.
func Load() (*Config, error) {
	cfg := &Config{
		ServerAddr:        ":8080",
		VaultSSHMount:     "ssh",
		GracePeriod:       15 * time.Minute,
		SessionGCInterval: 1 * time.Minute,
	}

	if v := os.Getenv("SERVER_PORT"); v != "" {
		cfg.ServerAddr = ":" + v
	}

	cfg.VaultAddr = os.Getenv("VAULT_ADDR")
	if cfg.VaultAddr == "" {
		return nil, fmt.Errorf("config: VAULT_ADDR environment variable is required")
	}

	cfg.VaultToken = Secret(os.Getenv("VAULT_TOKEN"))
	if cfg.VaultToken == "" {
		return nil, fmt.Errorf("config: VAULT_TOKEN environment variable is required")
	}

	if v := os.Getenv("VAULT_SSH_MOUNT"); v != "" {
		cfg.VaultSSHMount = v
	}

	cfg.VaultSSHRole = os.Getenv("VAULT_SSH_ROLE")
	if cfg.VaultSSHRole == "" {
		return nil, fmt.Errorf("config: VAULT_SSH_ROLE environment variable is required")
	}

	// CORS allowed origins — default to localhost dev server.
	if v := os.Getenv("CORS_ALLOWED_ORIGINS"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				cfg.AllowedOrigins = append(cfg.AllowedOrigins, trimmed)
			}
		}
	}
	if len(cfg.AllowedOrigins) == 0 {
		cfg.AllowedOrigins = []string{"http://localhost:5173"}
	}

	cfg.KnownHostsPath = os.Getenv("KNOWN_HOSTS_PATH")
	cfg.DBPath = os.Getenv("DB_PATH")

	return cfg, nil
}
