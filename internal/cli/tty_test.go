package cli

import (
	"os"
	"testing"

	"github.com/creack/pty"
)

func openDevNull(t *testing.T) *os.File {
	t.Helper()
	// /dev/null is not a terminal.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// TestShouldAllocateTTYFor_NoTerminal needs no pty, so it runs everywhere.
func TestShouldAllocateTTYFor_NoTerminal(t *testing.T) {
	devNull := openDevNull(t)
	if shouldAllocateTTYFor(devNull, devNull, devNull) {
		t.Error("shouldAllocateTTYFor() = true, want false when no stream is a terminal")
	}
}

func TestShouldAllocateTTYFor(t *testing.T) {
	// A pty slave is a terminal regardless of how the test itself is run,
	// so both branches are exercised even in CI where stdio is not a TTY.
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = tty.Close()
		_ = ptmx.Close()
	})
	devNull := openDevNull(t)

	tests := []struct {
		name                  string
		stdin, stdout, stderr *os.File
		want                  bool
	}{
		{"all terminals", tty, tty, tty, true},
		{"stdin not a terminal", devNull, tty, tty, false},
		{"stdout not a terminal", tty, devNull, tty, false},
		{"stderr not a terminal", tty, tty, devNull, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAllocateTTYFor(tt.stdin, tt.stdout, tt.stderr); got != tt.want {
				t.Errorf("shouldAllocateTTYFor() = %v, want %v", got, tt.want)
			}
		})
	}
}
