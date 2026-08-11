package cli

import (
	"os"

	"github.com/mattn/go-isatty"
)

// shouldAllocateTTY reports whether the CLI should request a TTY from ssh.
// A TTY is allocated only when stdin, stdout, and stderr are all connected
// to a terminal.
func shouldAllocateTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) &&
		isatty.IsTerminal(os.Stdout.Fd()) &&
		isatty.IsTerminal(os.Stderr.Fd())
}
