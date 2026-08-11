package cli

import (
	"os"

	"github.com/mattn/go-isatty"
)

// shouldAllocateTTY reports whether the CLI should request a TTY from ssh.
// A TTY is allocated only when stdin, stdout, and stderr are all connected
// to a terminal.
func shouldAllocateTTY() bool {
	return shouldAllocateTTYFor(os.Stdin, os.Stdout, os.Stderr)
}

// shouldAllocateTTYFor is the testable version of shouldAllocateTTY.
func shouldAllocateTTYFor(stdin, stdout, stderr *os.File) bool {
	return isatty.IsTerminal(stdin.Fd()) &&
		isatty.IsTerminal(stdout.Fd()) &&
		isatty.IsTerminal(stderr.Fd())
}
