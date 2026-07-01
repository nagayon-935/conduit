package sshconn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	dialTimeout    = 15 * time.Second
	defaultPTYRows = 24
	defaultPTYCols = 80
	termBaudRate   = 14400
	termType       = "xterm-256color"
)

// ErrPassphraseRequired is returned by buildAuthMethods when a user-supplied
// private key is encrypted but no passphrase was provided.
var ErrPassphraseRequired = errors.New("private key requires a passphrase")

// ConnectRequest carries all parameters needed to establish an SSH session.
type ConnectRequest struct {
	Host     string
	Port     int
	User     string
	AuthType string // "vault" | "password" | "pubkey"
	// vault
	PrivateKey  []byte // PEM-encoded ED25519 private key
	Certificate []byte // Vault-issued SSH certificate (OpenSSH format string as bytes)
	// password
	Password string
	// pubkey (user-provided)
	UserPrivateKey           []byte
	UserPrivateKeyPassphrase []byte // only used if UserPrivateKey is encrypted

	// ProxyJump (optional — zero JumpHost means no jump)
	JumpHost                     string
	JumpPort                     int
	JumpUser                     string
	JumpAuthType                 string // "vault" | "password" | "pubkey"
	JumpPrivateKey               []byte
	JumpCertificate              []byte
	JumpPassword                 string
	JumpUserPrivateKey           []byte
	JumpUserPrivateKeyPassphrase []byte // only used if JumpUserPrivateKey is encrypted
}

// ClearSecrets zero-fills all sensitive key and certificate slices in the request.
func (r *ConnectRequest) ClearSecrets() {
	clearSlice(r.PrivateKey)
	clearSlice(r.Certificate)
	clearSlice(r.UserPrivateKey)
	clearSlice(r.JumpPrivateKey)
	clearSlice(r.JumpCertificate)
	clearSlice(r.JumpUserPrivateKey)
}

func clearSlice(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// SSHDialer is the interface for dialing SSH connections.
type SSHDialer interface {
	// Dial opens an SSH connection and allocates a PTY session.
	// Returns: SSH client, SSH session, stdin writer, stdout reader, error.
	Dial(ctx context.Context, req ConnectRequest) (*ssh.Client, *ssh.Session, io.WriteCloser, io.Reader, error)
}

// Dialer is the concrete implementation of SSHDialer.
type Dialer struct {
	knownHostsPath string
}

// NewDialer constructs a Dialer. knownHostsPath may be empty, in which case
// host key verification is disabled with a warning (development only).
func NewDialer(knownHostsPath string) *Dialer {
	return &Dialer{knownHostsPath: knownHostsPath}
}

// hostKeyCallback returns an ssh.HostKeyCallback.
// If knownHostsPath is set, it uses knownhosts.New for strict verification.
// If empty, it falls back to InsecureIgnoreHostKey with a warning.
func (d *Dialer) hostKeyCallback() (ssh.HostKeyCallback, error) {
	if d.knownHostsPath == "" {
		slog.Warn("KNOWN_HOSTS_PATH is not set: SSH host key verification is disabled. " +
			"Set KNOWN_HOSTS_PATH to enable verification and prevent MITM attacks.")
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec
	}
	cb, err := knownhosts.New(d.knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("sshconn: load known_hosts %q: %w", d.knownHostsPath, err)
	}
	return cb, nil
}

// connWithCloser wraps a net.Conn and closes an additional io.Closer on Close.
// Used to ensure the ProxyJump client is cleaned up when the tunneled connection closes.
type connWithCloser struct {
	net.Conn
	extra io.Closer
}

func (c *connWithCloser) Close() error {
	return errors.Join(c.Conn.Close(), c.extra.Close())
}

// buildAuthMethods returns the appropriate ssh.AuthMethod slice for the given auth parameters.
func buildAuthMethods(authType, password string, privateKey, certificate, userPrivateKey, passphrase []byte) ([]ssh.AuthMethod, error) {
	switch authType {
	case "password":
		return []ssh.AuthMethod{ssh.Password(password)}, nil
	case "pubkey":
		signer, err := ssh.ParsePrivateKey(userPrivateKey)
		if err != nil {
			if !strings.Contains(err.Error(), "passphrase protected") {
				return nil, fmt.Errorf("parse user private key: %w", err)
			}
			if len(passphrase) == 0 {
				return nil, ErrPassphraseRequired
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(userPrivateKey, passphrase)
			if err != nil {
				return nil, fmt.Errorf("parse user private key: %w", err)
			}
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default: // "vault" or ""
		signer, err := buildCertSigner(privateKey, certificate)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}
}

// Dial connects to the SSH server described in req, optionally via a ProxyJump host,
// requests a PTY, and starts a shell.
func (d *Dialer) Dial(ctx context.Context, req ConnectRequest) (*ssh.Client, *ssh.Session, io.WriteCloser, io.Reader, error) {
	hkc, err := d.hostKeyCallback()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	authMethods, err := buildAuthMethods(req.AuthType, req.Password, req.PrivateKey, req.Certificate, req.UserPrivateKey, req.UserPrivateKeyPassphrase)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("sshconn: build auth methods: %w", err)
	}

	targetAddr := net.JoinHostPort(req.Host, fmt.Sprintf("%d", req.Port))

	sshCfg := &ssh.ClientConfig{
		User:            req.User,
		Auth:            authMethods,
		HostKeyCallback: hkc,
		Timeout:         dialTimeout,
	}

	var client *ssh.Client

	if req.JumpHost != "" {
		// ── ProxyJump path ──────────────────────────────────────────────
		jumpAuthMethods, err := buildAuthMethods(
			req.JumpAuthType, req.JumpPassword,
			req.JumpPrivateKey, req.JumpCertificate, req.JumpUserPrivateKey, req.JumpUserPrivateKeyPassphrase,
		)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("sshconn: build jump auth methods: %w", err)
		}

		jumpAddr := net.JoinHostPort(req.JumpHost, fmt.Sprintf("%d", req.JumpPort))
		jumpCfg := &ssh.ClientConfig{
			User:            req.JumpUser,
			Auth:            jumpAuthMethods,
			HostKeyCallback: hkc,
			Timeout:         dialTimeout,
		}

		type jumpResult struct {
			client *ssh.Client
			err    error
		}
		jumpCh := make(chan jumpResult, 1)
		go func() {
			jc, err := ssh.Dial("tcp", jumpAddr, jumpCfg)
			jumpCh <- jumpResult{jc, err}
		}()

		var jumpClient *ssh.Client
		select {
		case <-ctx.Done():
			// Drain the goroutine so a successful dial is properly closed.
			go func() {
				if res := <-jumpCh; res.client != nil {
					res.client.Close()
				}
			}()
			return nil, nil, nil, nil, fmt.Errorf("sshconn: context cancelled while dialing jump host: %w", ctx.Err())
		case res := <-jumpCh:
			if res.err != nil {
				return nil, nil, nil, nil, fmt.Errorf("sshconn: dial jump host %s: %w", jumpAddr, res.err)
			}
			jumpClient = res.client
		}

		// Open a TCP tunnel to the target host through the jump host.
		tunnel, err := jumpClient.Dial("tcp", targetAddr)
		if err != nil {
			jumpClient.Close()
			return nil, nil, nil, nil, fmt.Errorf("sshconn: tunnel to %s via jump: %w", targetAddr, err)
		}

		// Wrap so that closing the tunnel also closes the jump client.
		wrapped := &connWithCloser{Conn: tunnel, extra: jumpClient}

		// Perform the SSH handshake with the target over the tunnel.
		ncc, chans, reqs, err := ssh.NewClientConn(wrapped, targetAddr, sshCfg)
		if err != nil {
			wrapped.Close()
			return nil, nil, nil, nil, fmt.Errorf("sshconn: ssh handshake with %s via jump: %w", targetAddr, err)
		}

		client = ssh.NewClient(ncc, chans, reqs)
	} else {
		// ── Direct dial path ─────────────────────────────────────────────
		type dialResult struct {
			client *ssh.Client
			err    error
		}
		ch := make(chan dialResult, 1)
		go func() {
			c, err := ssh.Dial("tcp", targetAddr, sshCfg)
			ch <- dialResult{c, err}
		}()

		select {
		case <-ctx.Done():
			// Drain the goroutine so a successful dial is properly closed.
			go func() {
				if res := <-ch; res.client != nil {
					res.client.Close()
				}
			}()
			return nil, nil, nil, nil, fmt.Errorf("sshconn: context cancelled while dialing: %w", ctx.Err())
		case res := <-ch:
			if res.err != nil {
				return nil, nil, nil, nil, fmt.Errorf("sshconn: dial %s: %w", targetAddr, res.err)
			}
			client = res.client
		}
	}

	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, nil, nil, fmt.Errorf("sshconn: new session: %w", err)
	}

	// If anything fails after session creation, clean up both session and client.
	cleanup := func() { sess.Close(); client.Close() }

	// Request PTY with sane defaults.
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: termBaudRate,
		ssh.TTY_OP_OSPEED: termBaudRate,
	}
	if err := sess.RequestPty(termType, defaultPTYRows, defaultPTYCols, modes); err != nil {
		cleanup()
		return nil, nil, nil, nil, fmt.Errorf("sshconn: request PTY: %w", err)
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		cleanup()
		return nil, nil, nil, nil, fmt.Errorf("sshconn: stdin pipe: %w", err)
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, nil, nil, nil, fmt.Errorf("sshconn: stdout pipe: %w", err)
	}

	if err := sess.Shell(); err != nil {
		cleanup()
		return nil, nil, nil, nil, fmt.Errorf("sshconn: start shell: %w", err)
	}

	return client, sess, stdin, stdout, nil
}

// buildCertSigner constructs an ssh.Signer that authenticates with a Vault-issued certificate.
func buildCertSigner(privateKeyPEM []byte, certBytes []byte) (ssh.Signer, error) {
	signer, err := ssh.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	certStr := strings.TrimSpace(string(certBytes))
	if certStr == "" {
		return nil, fmt.Errorf("certificate is empty")
	}

	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(certStr))
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	cert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return nil, fmt.Errorf("parsed key is not an SSH certificate")
	}

	certSigner, err := ssh.NewCertSigner(cert, signer)
	if err != nil {
		return nil, fmt.Errorf("new cert signer: %w", err)
	}

	return certSigner, nil
}
