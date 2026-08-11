package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestParseTarget(t *testing.T) {
	// Set a user so tests don't depend on the environment.
	os.Setenv("USER", "testuser")
	defer os.Unsetenv("USER")

	tests := []struct {
		name       string
		input      string
		defaultPort int
		wantUser   string
		wantHost   string
		wantPort   int
		wantErr    bool
	}{
		{
			name:       "simple host",
			input:      "host.example.com",
			defaultPort: 22,
			wantUser:   "testuser",
			wantHost:   "host.example.com",
			wantPort:   22,
		},
		{
			name:       "user and host",
			input:      "alice@host.example.com",
			defaultPort: 22,
			wantUser:   "alice",
			wantHost:   "host.example.com",
			wantPort:   22,
		},
		{
			name:       "host with port",
			input:      "host.example.com:2222",
			defaultPort: 22,
			wantUser:   "testuser",
			wantHost:   "host.example.com",
			wantPort:   2222,
		},
		{
			name:       "user host port",
			input:      "alice@host.example.com:2222",
			defaultPort: 22,
			wantUser:   "alice",
			wantHost:   "host.example.com",
			wantPort:   2222,
		},
		{
			name:       "user with at in name",
			input:      "alice@domain@host.example.com",
			defaultPort: 22,
			wantUser:   "alice@domain",
			wantHost:   "host.example.com",
			wantPort:   22,
		},
		{
			name:       "empty host",
			input:      ":2222",
			defaultPort: 22,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, host, port, err := parseTarget(tt.input, tt.defaultPort)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseTarget(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if user != tt.wantUser {
				t.Errorf("user = %q, want %q", user, tt.wantUser)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
		})
	}
}

func TestValidateAuth(t *testing.T) {
	for _, auth := range []string{"vault", "password", "pubkey"} {
		if err := validateAuth(auth); err != nil {
			t.Errorf("validateAuth(%q) = %v, want nil", auth, err)
		}
	}
	if err := validateAuth("invalid"); err == nil {
		t.Error("validateAuth(invalid) = nil, want error")
	}
}

func TestCurrentUsername(t *testing.T) {
	t.Setenv("USER", "alice")
	t.Setenv("USERNAME", "")
	if u, err := currentUsername(); err != nil || u != "alice" {
		t.Errorf("currentUsername() = %q, %v; want alice, nil", u, err)
	}

	t.Setenv("USER", "")
	t.Setenv("USERNAME", "bob")
	if u, err := currentUsername(); err != nil || u != "bob" {
		t.Errorf("currentUsername() = %q, %v; want bob, nil", u, err)
	}

	t.Setenv("USER", "")
	t.Setenv("USERNAME", "")
	if _, err := currentUsername(); err == nil {
		t.Error("currentUsername() = nil, want error")
	}
}

func TestNewLogger(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "silent", "unknown"} {
		logger := newLogger(level)
		if logger == nil {
			t.Errorf("newLogger(%q) returned nil", level)
		}
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"vault signing", &exitError{err: errors.New("vault signing failed")}, exitVaultError},
		{"key generation", errors.New("generate key pair failed"), exitKeyError},
		{"ssh not found", errors.New("ssh command not found in PATH"), exitSSHNotFound},
		{"context canceled", context.Canceled, exitInterrupted},
		{"other", errors.New("some other error"), exitConfigError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ee := classifyError(tt.err)
			var got *exitError
			if !errors.As(ee, &got) {
				t.Fatalf("classifyError did not return *exitError")
			}
			if got.code != tt.want {
				t.Errorf("code = %d, want %d", got.code, tt.want)
			}
		})
	}
}

func TestExitError(t *testing.T) {
	inner := errors.New("inner error")
	ee := &exitError{code: 42, err: inner}
	if ee.Error() != "inner error" {
		t.Errorf("Error() = %q, want %q", ee.Error(), "inner error")
	}
	if !errors.Is(ee, inner) {
		t.Error("exitError should wrap inner error")
	}
}

func TestBindPFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("foo", "", "")
	bindPFlag(cmd, "test.key", "foo")
	if viper.GetString("test.key") != "" {
		t.Error("expected default empty value")
	}
}

func TestBindPFlag_PanicOnMissingFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing flag")
		}
	}()
	bindPFlag(cmd, "test.key", "missing")
}

func TestGetStringFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("foo", "default", "")
	if got := getStringFlag(cmd, "foo"); got != "default" {
		t.Errorf("getStringFlag = %q, want default", got)
	}
	_ = cmd.Flags().Set("foo", "set")
	if got := getStringFlag(cmd, "foo"); got != "set" {
		t.Errorf("getStringFlag = %q, want set", got)
	}
}
