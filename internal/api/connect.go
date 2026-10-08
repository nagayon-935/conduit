package api

import (
	"context"
	"crypto/x509"
	"errors"
	"strings"

	"github.com/nagayon-935/conduit/internal/sshconn"
)

// classifyDialError maps a (possibly deeply wrapped) SSH dial error to a short,
// user-facing message. Raw transport errors may contain peer-provided secrets,
// so only this classification is persisted or returned.
func classifyDialError(err error) string {
	switch {
	case errors.Is(err, sshconn.ErrPassphraseRequired):
		return "This private key requires a passphrase."
	case errors.Is(err, x509.IncorrectPasswordError):
		return "Incorrect passphrase for the private key."
	case strings.Contains(err.Error(), "unable to authenticate"):
		return "Authentication failed. Check your username, password, or key."
	case strings.Contains(err.Error(), "connection refused"):
		return "Connection refused."
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(err.Error(), "i/o timeout"):
		return "Connection timed out."
	case strings.Contains(err.Error(), "no route to host"):
		return "Could not reach the host."
	default:
		return "SSH connection failed. See connection logs for details."
	}
}
