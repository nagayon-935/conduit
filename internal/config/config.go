package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all application configuration values.
type Config struct {
	DevHTTP           bool
	PublicURL         string
	AllowedCIDRs      []string
	TrustedProxyCIDRs []string
	ServerAddr        string
	VaultAddr         string
	VaultToken        Secret
	VaultSSHMount     string
	VaultSSHRole      string
	GracePeriod       time.Duration
	SessionGCInterval time.Duration
	// KnownHostsPath is the path to the SSH known_hosts file used for host key verification.
	// Loaded from KNOWN_HOSTS_PATH.
	KnownHostsPath string
	// DBPath is the mandatory identity and audit database (default ./data/conduit.db).
	DBPath string
	// RecordingEnabled controls whether SSH sessions are recorded.
	// Loaded from RECORDING_ENABLED (any non-empty value enables it).
	RecordingEnabled bool
	// RecordingDir is the directory where .cast recording files are stored.
	// Loaded from RECORDING_DIR. Defaults to "./recordings".
	RecordingDir string
	// IdleTimeout closes a session when no stdin has been forwarded to the SSH
	// process for this duration, even while WebSocket connections remain open.
	// Loaded from SESSION_IDLE_TIMEOUT (Go duration string). 0 disables it.
	IdleTimeout time.Duration
}

// Load reads configuration from environment variables and applies defaults.
func Load() (*Config, error) {
	cfg := &Config{
		ServerAddr:        ":8080",
		VaultSSHMount:     "ssh",
		GracePeriod:       15 * time.Minute,
		SessionGCInterval: 1 * time.Minute,
		IdleTimeout:       30 * time.Minute,
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

	cfg.KnownHostsPath = os.Getenv("KNOWN_HOSTS_PATH")
	cfg.DBPath = os.Getenv("DB_PATH")
	if cfg.DBPath == "" {
		cfg.DBPath = "./data/conduit.db"
	}
	cfg.DevHTTP = os.Getenv("CONDUIT_DEV_HTTP") == "true"
	cfg.PublicURL = strings.TrimRight(os.Getenv("PUBLIC_URL"), "/")
	if v := os.Getenv("SSH_ALLOWED_CIDRS"); v != "" {
		for _, c := range strings.Split(v, ",") {
			cfg.AllowedCIDRs = append(cfg.AllowedCIDRs, strings.TrimSpace(c))
		}
	}
	if v := os.Getenv("TRUSTED_PROXY_CIDRS"); v != "" {
		for _, c := range strings.Split(v, ",") {
			cfg.TrustedProxyCIDRs = append(cfg.TrustedProxyCIDRs, strings.TrimSpace(c))
		}
	}
	cfg.RecordingEnabled = os.Getenv("RECORDING_ENABLED") != ""
	cfg.RecordingDir = os.Getenv("RECORDING_DIR")
	if cfg.RecordingDir == "" {
		cfg.RecordingDir = "./recordings"
	}

	if v := os.Getenv("SESSION_IDLE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return nil, fmt.Errorf("config: SESSION_IDLE_TIMEOUT must be a non-negative Go duration, got %q", v)
		}
		cfg.IdleTimeout = d
	}
	for key, dst := range map[string]*time.Duration{"GRACE_PERIOD": &cfg.GracePeriod, "SESSION_GC_INTERVAL": &cfg.SessionGCInterval} {
		if v := os.Getenv(key); v != "" {
			d, e := time.ParseDuration(v)
			if e != nil || d < time.Second || d > 2*time.Hour || (key == "GRACE_PERIOD" && d < time.Minute) {
				return nil, fmt.Errorf("config: invalid %s duration", key)
			}
			*dst = d
		}
	}
	if cfg.IdleTimeout > 24*time.Hour || (cfg.IdleTimeout > 0 && cfg.IdleTimeout < time.Minute) {
		return nil, fmt.Errorf("config: SESSION_IDLE_TIMEOUT must be 0 or between 1m and 24h")
	}

	return cfg, nil
}
