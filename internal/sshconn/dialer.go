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
	DialIP     string // pinned, policy-checked address; Host remains the known_hosts identity
	JumpDialIP string
	Host       string
	Port       int
	User       string
	AuthType   string // "vault" | "password" | "pubkey"
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
	clearSlice(r.UserPrivateKeyPassphrase)
	clearSlice(r.JumpUserPrivateKeyPassphrase)
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
func (d *Dialer) Dial(ctx context.Context, req ConnectRequest) (client *ssh.Client, sess *ssh.Session, stdin io.WriteCloser, stdout io.Reader, err error) {
	// Bound the entire setup, including handshakes, ProxyJump and PTY/shell
	// requests. ssh.ClientConfig.Timeout only limits the TCP connection.
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
	}()
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

		jumpDialAddr := jumpAddr
		if req.JumpDialIP != "" {
			jumpDialAddr = net.JoinHostPort(req.JumpDialIP, fmt.Sprint(req.JumpPort))
		}
		jumpClient, err := dialPinnedSSHClient(ctx, jumpDialAddr, jumpAddr, jumpCfg)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("sshconn: dial jump host %s: %w", jumpAddr, err)
		}
		stopJumpCancel := context.AfterFunc(ctx, func() { _ = jumpClient.Close() })
		defer stopJumpCancel()

		// Open a TCP tunnel to the target host through the jump host.
		tunnelAddr := targetAddr
		if req.DialIP != "" {
			tunnelAddr = net.JoinHostPort(req.DialIP, fmt.Sprint(req.Port))
		}
		tunnel, err := jumpClient.DialContext(ctx, "tcp", tunnelAddr)
		if err != nil {
			jumpClient.Close()
			return nil, nil, nil, nil, fmt.Errorf("sshconn: tunnel to %s via jump: %w", targetAddr, err)
		}

		// Wrap so that closing the tunnel also closes the jump client.
		wrapped := &connWithCloser{Conn: tunnel, extra: jumpClient}

		// Perform the target handshake with the same cancellation support.
		client, err = handshakeSSH(ctx, wrapped, targetAddr, sshCfg)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("sshconn: ssh handshake with %s via jump: %w", targetAddr, err)
		}
	} else {
		dialAddr := targetAddr
		if req.DialIP != "" {
			dialAddr = net.JoinHostPort(req.DialIP, fmt.Sprint(req.Port))
		}
		client, err = dialPinnedSSHClient(ctx, dialAddr, targetAddr, sshCfg)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("sshconn: dial %s: %w", targetAddr, err)
		}
	}

	// Closing the transport interrupts channel-open and request/reply waits.
	// Detach this callback before returning ownership of a successful session.
	setupClient := client
	stopCancel := context.AfterFunc(ctx, func() { _ = setupClient.Close() })
	defer stopCancel()

	sess, err = client.NewSession()
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

	stdin, err = sess.StdinPipe()
	if err != nil {
		cleanup()
		return nil, nil, nil, nil, fmt.Errorf("sshconn: stdin pipe: %w", err)
	}

	stdout, err = sess.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, nil, nil, nil, fmt.Errorf("sshconn: stdout pipe: %w", err)
	}

	if err := sess.Shell(); err != nil {
		cleanup()
		return nil, nil, nil, nil, fmt.Errorf("sshconn: start shell: %w", err)
	}

	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, nil, nil, nil, fmt.Errorf("sshconn: setup cancelled: %w", err)
	}
	return client, sess, stdin, stdout, nil
}

// dialSSHClient uses a cancellable TCP dial rather than leaving ssh.Dial
// running in a goroutine after the caller has already returned.
func dialSSHClient(ctx context.Context, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	return dialPinnedSSHClient(ctx, addr, addr, cfg)
}
func dialPinnedSSHClient(ctx context.Context, dialAddr, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", dialAddr)
	if err != nil {
		return nil, err
	}
	return handshakeSSH(ctx, conn, addr, cfg)
}

// handshakeSSH owns conn on entry and closes it on every failed handshake.
func handshakeSSH(ctx context.Context, conn net.Conn, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()
	ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	client := ssh.NewClient(ncc, chans, reqs)
	if err := ctx.Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
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
