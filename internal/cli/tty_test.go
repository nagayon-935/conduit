package cli

import (
	"os"
	"testing"
)

func TestShouldAllocateTTYFor(t *testing.T) {
	// /dev/null is not a terminal.
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer devNull.Close()

	// Use the current process stdin/stdout/stderr for terminal detection.
	// This test simply exercises both true and false branches by mixing
	// /dev/null with the real descriptors.
	if shouldAllocateTTYFor(devNull, os.Stdout, os.Stderr) {
		t.Error("expected false when stdin is not a terminal")
	}
	if shouldAllocateTTYFor(os.Stdin, devNull, os.Stderr) {
		t.Error("expected false when stdout is not a terminal")
	}
	if shouldAllocateTTYFor(os.Stdin, os.Stdout, devNull) {
		t.Error("expected false when stderr is not a terminal")
	}
}
