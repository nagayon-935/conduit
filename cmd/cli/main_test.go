package main

import (
	"os"
	"testing"
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
