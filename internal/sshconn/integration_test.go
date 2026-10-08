package sshconn

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// generateEd25519Key returns a raw ED25519 private key.
func generateEd25519Key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generateEd25519Key: %v", err)
	}
	return priv
}

// marshalPrivKeyPEM converts an ED25519 private key to OpenSSH PEM bytes.
func marshalPrivKeyPEM(t *testing.T, priv ed25519.PrivateKey) []byte {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshalPrivKeyPEM: %v", err)
	}
	return pem.EncodeToMemory(block)
}

// signCert signs pubKey with caPriv and returns an OpenSSH user certificate string.
func signCert(t *testing.T, pub ssh.PublicKey, caPriv ssh.Signer, principals []string) string {
	t.Helper()
	cert := &ssh.Certificate{
		Key:             pub,
		CertType:        ssh.UserCert,
		ValidPrincipals: principals,
		ValidAfter:      0,
		ValidBefore:     ssh.CertTimeInfinity,
	}
	if err := cert.SignCert(rand.Reader, caPriv); err != nil {
		t.Fatalf("signCert: %v", err)
	}
	return string(ssh.MarshalAuthorizedKey(cert))
}

// startTestSSHServer spins up an in-process SSH server that accepts certificates
// signed by caPubKey. Returns the listening port and a cleanup function.
func startTestSSHServer(t *testing.T, caPubKey ssh.PublicKey) (port int, cleanup func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startTestSSHServer listen: %v", err)
	}
	port = listener.Addr().(*net.TCPAddr).Port

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			cert, ok := key.(*ssh.Certificate)
			if !ok {
				return nil, fmt.Errorf("not a certificate")
			}
			checker := &ssh.CertChecker{
				IsUserAuthority: func(auth ssh.PublicKey) bool {
					return bytes.Equal(auth.Marshal(), caPubKey.Marshal())
				},
			}
			return checker.Authenticate(conn, cert)
		},
	}

	hostKey := generateEd25519Key(t)
	hostSigner, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatalf("startTestSSHServer: host signer: %v", err)
	}
	config.AddHostKey(hostSigner)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleSSHConn(conn, config)
		}
	}()

	return port, func() { listener.Close() }
}

func handleSSHConn(conn net.Conn, config *ssh.ServerConfig) {
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unknown")
			continue
		}
		ch, requests, err := newChan.Accept()
		if err != nil {
			return
		}
		go func(ch ssh.Channel, requests <-chan *ssh.Request) {
			defer ch.Close()
			for req := range requests {
				switch req.Type {
				case "pty-req":
					_ = req.Reply(true, nil)
				case "shell":
					_ = req.Reply(true, nil)
					// Echo back whatever is written.
					go io.Copy(ch, ch)
				case "window-change":
					_ = req.Reply(true, nil)
				default:
					_ = req.Reply(false, nil)
				}
			}
		}(ch, requests)
	}
}

func TestDial_Success(t *testing.T) {
	t.Parallel()

	// Generate CA key.
	caPriv := generateEd25519Key(t)
	caSigner, err := ssh.NewSignerFromKey(caPriv)
	if err != nil {
		t.Fatalf("ca signer: %v", err)
	}
	caPub := caSigner.PublicKey()

	port, cleanup := startTestSSHServer(t, caPub)
	defer cleanup()

	// Generate user key pair.
	userPriv := generateEd25519Key(t)
	userPrivPEM := marshalPrivKeyPEM(t, userPriv)
	userSSHPub, err := ssh.NewPublicKey(userPriv.Public())
	if err != nil {
		t.Fatalf("user pub: %v", err)
	}

	// Sign the user public key with the CA.
	certStr := signCert(t, userSSHPub, caSigner, []string{"testuser"})

	d := NewDialer("") // empty = insecure for tests
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, sess, stdin, stdout, err := d.Dial(ctx, ConnectRequest{
		Host:        "127.0.0.1",
		Port:        port,
		User:        "testuser",
		PrivateKey:  userPrivPEM,
		Certificate: []byte(certStr),
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()
	defer sess.Close()
	cancel() // Setup cancellation must not close a successfully returned session.

	// Write something to stdin. The echo server copies stdin → stdout.
	msg := "hello\n"
	if _, err := io.WriteString(stdin, msg); err != nil {
		t.Fatalf("write stdin: %v", err)
	}

	echo := make(chan error, 1)
	go func() {
		buf := make([]byte, len(msg))
		_, err := io.ReadFull(stdout, buf)
		if err == nil && string(buf) != msg {
			err = fmt.Errorf("echo = %q, want %q", buf, msg)
		}
		echo <- err
	}()
	select {
	case err := <-echo:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("session stopped working after setup context cancellation")
	}
}

func TestDial_InvalidCertificate(t *testing.T) {
	t.Parallel()

	// Generate a CA that the server trusts.
	caPriv := generateEd25519Key(t)
	caSigner, err := ssh.NewSignerFromKey(caPriv)
	if err != nil {
		t.Fatalf("ca signer: %v", err)
	}
	caPub := caSigner.PublicKey()

	port, cleanup := startTestSSHServer(t, caPub)
	defer cleanup()

	// Generate a different (untrusted) CA to sign the user certificate.
	untrustedPriv := generateEd25519Key(t)
	untrustedSigner, err := ssh.NewSignerFromKey(untrustedPriv)
	if err != nil {
		t.Fatalf("untrusted signer: %v", err)
	}

	userPriv := generateEd25519Key(t)
	userPrivPEM := marshalPrivKeyPEM(t, userPriv)
	userSSHPub, err := ssh.NewPublicKey(userPriv.Public())
	if err != nil {
		t.Fatalf("user pub: %v", err)
	}

	// Sign with untrusted CA.
	certStr := signCert(t, userSSHPub, untrustedSigner, []string{"testuser"})

	d := NewDialer("") // empty = insecure for tests
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _, _, _, err = d.Dial(ctx, ConnectRequest{
		Host:        "127.0.0.1",
		Port:        port,
		User:        "testuser",
		PrivateKey:  userPrivPEM,
		Certificate: []byte(certStr),
	})
	if err == nil {
		t.Fatal("expected authentication failure, got nil")
	}
}

func TestDial_ConnectionRefused(t *testing.T) {
	t.Parallel()

	// Find a port with nothing listening.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // Close immediately so the port is free but nothing listens.

	// Generate a minimal key + cert.
	caPriv := generateEd25519Key(t)
	caSigner, err := ssh.NewSignerFromKey(caPriv)
	if err != nil {
		t.Fatalf("ca signer: %v", err)
	}
	userPriv := generateEd25519Key(t)
	userPrivPEM := marshalPrivKeyPEM(t, userPriv)
	userSSHPub, err := ssh.NewPublicKey(userPriv.Public())
	if err != nil {
		t.Fatalf("user pub: %v", err)
	}
	certStr := signCert(t, userSSHPub, caSigner, []string{"testuser"})

	d := NewDialer("") // empty = insecure for tests
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, _, _, err = d.Dial(ctx, ConnectRequest{
		Host:        "127.0.0.1",
		Port:        port,
		User:        "testuser",
		PrivateKey:  userPrivPEM,
		Certificate: []byte(certStr),
	})
	if err == nil {
		t.Fatal("expected connection refused error, got nil")
	}
}

func TestDial_ContextTimeout(t *testing.T) {
	t.Parallel()

	// Start a TCP listener that hangs (never completes the SSH handshake).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	// Accept connections but don't speak SSH.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold the connection open without sending anything.
			go func(c net.Conn) {
				defer c.Close()
				time.Sleep(5 * time.Second)
			}(conn)
		}
	}()

	caPriv := generateEd25519Key(t)
	caSigner, err := ssh.NewSignerFromKey(caPriv)
	if err != nil {
		t.Fatalf("ca signer: %v", err)
	}
	userPriv := generateEd25519Key(t)
	userPrivPEM := marshalPrivKeyPEM(t, userPriv)
	userSSHPub, err := ssh.NewPublicKey(userPriv.Public())
	if err != nil {
		t.Fatalf("user pub: %v", err)
	}
	certStr := signCert(t, userSSHPub, caSigner, []string{"testuser"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	d := NewDialer("") // empty = insecure for tests
	_, _, _, _, err = d.Dial(ctx, ConnectRequest{
		Host:        "127.0.0.1",
		Port:        port,
		User:        "testuser",
		PrivateKey:  userPrivPEM,
		Certificate: []byte(certStr),
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestBuildCertSigner_ValidCert(t *testing.T) {
	t.Parallel()

	caPriv := generateEd25519Key(t)
	caSigner, err := ssh.NewSignerFromKey(caPriv)
	if err != nil {
		t.Fatalf("ca signer: %v", err)
	}

	userPriv := generateEd25519Key(t)
	userPrivPEM := marshalPrivKeyPEM(t, userPriv)
	userSSHPub, err := ssh.NewPublicKey(userPriv.Public())
	if err != nil {
		t.Fatalf("user pub: %v", err)
	}
	certStr := signCert(t, userSSHPub, caSigner, []string{"testuser"})

	signer, err := buildCertSigner(userPrivPEM, []byte(certStr))
	if err != nil {
		t.Fatalf("buildCertSigner: %v", err)
	}
	if signer == nil {
		t.Fatal("buildCertSigner returned nil signer")
	}
}

func TestBuildCertSigner_EmptyCert(t *testing.T) {
	t.Parallel()

	userPriv := generateEd25519Key(t)
	userPrivPEM := marshalPrivKeyPEM(t, userPriv)

	_, err := buildCertSigner(userPrivPEM, []byte(""))
	if err == nil {
		t.Fatal("expected error for empty certificate, got nil")
	}
}

// TestBuildCertSigner_InvalidPrivateKey は PEM でない秘密鍵を渡した場合に
// エラーが返ることを検証する。
func TestBuildCertSigner_InvalidPrivateKey(t *testing.T) {
	t.Parallel()

	_, err := buildCertSigner([]byte("this is not a pem block"), []byte("some-cert"))
	if err == nil {
		t.Fatal("expected error for invalid private key PEM, got nil")
	}
}

// TestBuildCertSigner_InvalidCertString は空でないが不正な証明書文字列を渡した場合に
// エラーが返ることを検証する。
func TestBuildCertSigner_InvalidCertString(t *testing.T) {
	t.Parallel()

	userPriv := generateEd25519Key(t)
	userPrivPEM := marshalPrivKeyPEM(t, userPriv)

	_, err := buildCertSigner(userPrivPEM, []byte("not-a-valid-ssh-cert-string"))
	if err == nil {
		t.Fatal("expected error for invalid certificate string, got nil")
	}
}

// TestBuildCertSigner_PlainPublicKey は証明書ではなく通常の公開鍵を渡した場合に
// "not an SSH certificate" エラーが返ることを検証する。
func TestBuildCertSigner_PlainPublicKey(t *testing.T) {
	t.Parallel()

	userPriv := generateEd25519Key(t)
	userPrivPEM := marshalPrivKeyPEM(t, userPriv)

	// 通常の公開鍵（証明書ではない）を authorized_keys 形式で生成する
	sshPub, err := ssh.NewPublicKey(userPriv.Public())
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	plainPubKey := string(ssh.MarshalAuthorizedKey(sshPub))

	_, err = buildCertSigner(userPrivPEM, []byte(plainPubKey))
	if err == nil {
		t.Fatal("expected error when certificate is a plain public key, got nil")
	}
}
