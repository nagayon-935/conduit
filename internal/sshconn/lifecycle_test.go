package sshconn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// serveSetupStage accepts one connection and stalls at a selected setup stage.
// reached lets the test cancel only after the operation is actually blocked.
func serveSetupStage(t *testing.T, stage string) (int, <-chan struct{}, <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	signer, err := ssh.NewSignerFromKey(generateEd25519Key(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.AddHostKey(signer)
	reached, closed := make(chan struct{}), make(chan struct{})
	accepted := make(chan net.Conn, 1)
	go func() {
		defer close(closed)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- conn
		defer conn.Close()
		if stage == "handshake" {
			close(reached)
			_, _ = io.Copy(io.Discard, conn)
			return
		}
		server, chans, reqs, err := ssh.NewServerConn(conn, cfg)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(reqs)
		newChan, ok := <-chans
		if !ok {
			return
		}
		if stage == "proxy" {
			var addr struct {
				Host       string
				Port       uint32
				OriginHost string
				OriginPort uint32
			}
			if err := ssh.Unmarshal(newChan.ExtraData(), &addr); err != nil {
				return
			}
			target, err := net.Dial("tcp", net.JoinHostPort(addr.Host, fmt.Sprint(addr.Port)))
			if err != nil {
				_ = newChan.Reject(ssh.ConnectionFailed, err.Error())
				return
			}
			defer target.Close()
			ch, requests, err := newChan.Accept()
			if err != nil {
				return
			}
			defer ch.Close()
			close(reached)
			go ssh.DiscardRequests(requests)
			go func() { _, _ = io.Copy(ch, target) }()
			_, _ = io.Copy(target, ch)
			return
		}
		if stage == "session" || stage == "tunnel" {
			close(reached)
			_ = server.Wait() // never reply to the channel-open request
			return
		}
		ch, requests, err := newChan.Accept()
		if err != nil {
			return
		}
		defer ch.Close()
		if stage == "target-handshake" {
			close(reached)
			go ssh.DiscardRequests(requests)
			_, _ = io.Copy(io.Discard, ch)
			return
		}
		for req := range requests {
			if req.Type == stage {
				close(reached)
				_ = server.Wait() // never reply to the PTY or shell request
				return
			}
			_ = req.Reply(true, nil)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		select {
		case conn := <-accepted:
			_ = conn.Close()
		default:
		}
		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Error("test SSH server did not stop")
		}
	})
	return ln.Addr().(*net.TCPAddr).Port, reached, closed
}

func TestDial_CancelClosesSetupConnection(t *testing.T) {
	for _, stage := range []string{"handshake", "session", "pty-req", "shell", "tunnel", "target-handshake"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			port, reached, closed := serveSetupStage(t, stage)
			req := ConnectRequest{Host: "127.0.0.1", Port: port, User: "user", AuthType: "password", Password: "test"}
			if stage == "tunnel" || stage == "target-handshake" {
				req.JumpHost, req.JumpPort, req.JumpUser = req.Host, port, req.User
				req.JumpAuthType, req.JumpPassword = "password", "test"
				req.Host, req.Port = "target.example", 22
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				client, sess, _, _, err := NewDialer("").Dial(ctx, req)
				if sess != nil {
					_ = sess.Close()
				}
				if client != nil {
					_ = client.Close()
				}
				result <- err
			}()
			select {
			case <-reached:
			case <-time.After(time.Second):
				t.Fatal("dial did not reach " + stage)
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("Dial error = %v, want context.Canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Dial did not return after cancellation")
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("SSH connection remains open after cancellation")
			}
		})
	}
}

func TestDial_ContextDeadlineClosesHandshake(t *testing.T) {
	t.Parallel()
	port, _, closed := serveSetupStage(t, "handshake")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, _, _, err := NewDialer("").Dial(ctx, ConnectRequest{
		Host: "127.0.0.1", Port: port, User: "user", AuthType: "password", Password: "test",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Dial error = %v, want deadline exceeded", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("SSH handshake connection remains open after deadline")
	}
}

func TestDial_ProxyJumpSuccessOutlivesContext(t *testing.T) {
	t.Parallel()
	caSigner, err := ssh.NewSignerFromKey(generateEd25519Key(t))
	if err != nil {
		t.Fatal(err)
	}
	port, cleanup := startTestSSHServer(t, caSigner.PublicKey())
	defer cleanup()
	jumpPort, _, jumpClosed := serveSetupStage(t, "proxy")
	key := generateEd25519Key(t)
	pub, err := ssh.NewPublicKey(key.Public())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, sess, stdin, stdout, err := NewDialer("").Dial(ctx, ConnectRequest{
		Host: "127.0.0.1", Port: port, User: "testuser",
		PrivateKey: marshalPrivKeyPEM(t, key), Certificate: []byte(signCert(t, pub, caSigner, []string{"testuser"})),
		JumpHost: "127.0.0.1", JumpPort: jumpPort, JumpUser: "jumpuser", JumpAuthType: "password", JumpPassword: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	defer sess.Close()
	cancel()
	echo := make(chan error, 1)
	go func() {
		_, err := io.WriteString(stdin, "hello")
		if err == nil {
			buf := make([]byte, 5)
			_, err = io.ReadFull(stdout, buf)
			if err == nil && string(buf) != "hello" {
				err = fmt.Errorf("unexpected echo %q", buf)
			}
		}
		echo <- err
	}()
	select {
	case err := <-echo:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("ProxyJump session stopped working after setup context cancellation")
	}
	_ = sess.Close()
	_ = client.Close()
	select {
	case <-jumpClosed:
	case <-time.After(time.Second):
		t.Fatal("jump connection remains open after target client closes")
	}
}
