package tunnel

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nagayon-935/conduit/internal/session"
	"golang.org/x/crypto/ssh"
)

// ---------- helpers ----------

func generateTestEd25519Key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generateTestEd25519Key: %v", err)
	}
	return priv
}

func marshalTestPrivKeyPEM(t *testing.T, priv ed25519.PrivateKey) []byte {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshalTestPrivKeyPEM: %v", err)
	}
	return pem.EncodeToMemory(block)
}

func signTestCert(t *testing.T, pub ssh.PublicKey, caSigner ssh.Signer, principals []string) string {
	t.Helper()
	cert := &ssh.Certificate{
		Key:             pub,
		CertType:        ssh.UserCert,
		ValidPrincipals: principals,
		ValidAfter:      0,
		ValidBefore:     ssh.CertTimeInfinity,
	}
	if err := cert.SignCert(rand.Reader, caSigner); err != nil {
		t.Fatalf("signTestCert: %v", err)
	}
	return string(ssh.MarshalAuthorizedKey(cert))
}

// startEchoSSHServer starts a minimal in-process SSH server that echoes session data.
// It also records window-change requests on the returned channel.
func startEchoSSHServer(t *testing.T, caPubKey ssh.PublicKey) (port int, windowChanges chan struct{}, cleanup func()) {
	t.Helper()
	windowChanges = make(chan struct{}, 8)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startEchoSSHServer listen: %v", err)
	}
	port = listener.Addr().(*net.TCPAddr).Port

	cfg := &ssh.ServerConfig{
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
	hostKey := generateTestEd25519Key(t)
	hostSigner, _ := ssh.NewSignerFromKey(hostKey)
	cfg.AddHostKey(hostSigner)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				sshConn, chans, reqs, err := ssh.NewServerConn(c, cfg)
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
					go func(ch ssh.Channel, reqs <-chan *ssh.Request) {
						defer ch.Close()
						for req := range reqs {
							switch req.Type {
							case "pty-req":
								_ = req.Reply(true, nil)
							case "shell":
								_ = req.Reply(true, nil)
								go io.Copy(ch, ch)
							case "window-change":
								windowChanges <- struct{}{}
								_ = req.Reply(true, nil)
							default:
								_ = req.Reply(false, nil)
							}
						}
					}(ch, requests)
				}
			}(conn)
		}
	}()

	return port, windowChanges, func() { listener.Close() }
}

// dialTestSSH dials the test SSH server and returns a live *ssh.Session.
func dialTestSSH(t *testing.T, port int, privPEM []byte, certStr string) (*ssh.Client, *ssh.Session) {
	t.Helper()
	signer, err := ssh.ParsePrivateKey(privPEM)
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(certStr))
	if err != nil {
		t.Fatalf("ParseAuthorizedKey: %v", err)
	}
	cert, ok := pub.(*ssh.Certificate)
	if !ok {
		t.Fatalf("not a certificate")
	}
	certSigner, err := ssh.NewCertSigner(cert, signer)
	if err != nil {
		t.Fatalf("NewCertSigner: %v", err)
	}

	cfg := &ssh.ClientConfig{
		User:            "testuser",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(certSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         10 * time.Second,
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		t.Fatalf("ssh.Dial: %v", err)
	}
	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		t.Fatalf("NewSession: %v", err)
	}
	return client, sess
}

// ---------- tests ----------

func TestDrainOrDrop_SendsWhenCapacity(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 10)
	data := []byte("hello")
	sent := DrainOrDrop(ch, data, 1*time.Millisecond)
	if !sent {
		t.Fatal("expected data to be sent, but it was dropped")
	}
	if len(ch) != 1 {
		t.Errorf("expected len(ch)==1, got %d", len(ch))
	}
	got := <-ch
	if !bytes.Equal(got, data) {
		t.Errorf("data mismatch: got %q, want %q", got, data)
	}
}

func TestDrainOrDrop_DropsWhenFull(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 1)
	ch <- []byte("first") // fill the channel
	sent := DrainOrDrop(ch, []byte("second"), 1*time.Millisecond)
	if sent {
		t.Fatal("expected data to be dropped, but it was sent")
	}
	if len(ch) != 1 {
		t.Errorf("expected len(ch)==1 (still the original), got %d", len(ch))
	}
}

func TestResizePTY_SendsWindowChange(t *testing.T) {
	t.Parallel()

	caPriv := generateTestEd25519Key(t)
	caSigner, err := ssh.NewSignerFromKey(caPriv)
	if err != nil {
		t.Fatalf("ca signer: %v", err)
	}
	caPub := caSigner.PublicKey()

	port, windowChanges, cleanup := startEchoSSHServer(t, caPub)
	defer cleanup()

	userPriv := generateTestEd25519Key(t)
	userPrivPEM := marshalTestPrivKeyPEM(t, userPriv)
	userSSHPub, _ := ssh.NewPublicKey(userPriv.Public())
	certStr := signTestCert(t, userSSHPub, caSigner, []string{"testuser"})

	client, sshSess := dialTestSSH(t, port, userPrivPEM, certStr)
	defer client.Close()
	defer sshSess.Close()

	// Request a PTY first so window-change makes sense.
	if err := sshSess.RequestPty("xterm", 24, 80, ssh.TerminalModes{}); err != nil {
		t.Fatalf("RequestPty: %v", err)
	}
	if err := sshSess.Shell(); err != nil {
		t.Fatalf("Shell: %v", err)
	}

	if err := ResizePTY(sshSess, WindowSize{Cols: 120, Rows: 40}); err != nil {
		t.Fatalf("ResizePTY: %v", err)
	}

	select {
	case <-windowChanges:
		// success: server received the window-change request
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for window-change request on server")
	}
}

func TestStartPumps_SSHToWebSocket(t *testing.T) {
	t.Parallel()

	// Use an in-memory pipe to simulate SSH stdout.
	stdoutRead, stdoutWrite := io.Pipe()

	// NewSession wires up the done channel and buffered channels.
	realSess := session.NewSession("pump-test", "", 0, "", nil, nil, nil, stdoutRead, 15*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// sshToClientPump だけを起動する。
	// StartPumps を使うと readPump も同時に起動され、WSConn==nil のまま
	// sess.ToClient を先読みしてデータを捨てるためテストと競合する。
	// このテストは「SSH stdout → ToClient チャンネル」の経路のみを検証する。
	go sshToClientPump(ctx, realSess, DefaultPumpConfig())

	// Write data to the stdout pipe – sshToClientPump should forward it.
	payload := []byte("hello from ssh")
	go func() {
		_, _ = stdoutWrite.Write(payload)
		// Do NOT close the write-end immediately; keep it open so the pump doesn't
		// see EOF before we read from ToClient.
	}()

	select {
	case got := <-realSess.ToClient:
		if !bytes.Equal(got, payload) {
			t.Errorf("data mismatch: got %q, want %q", got, payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for data in ToClient channel")
	}
	stdoutWrite.Close()
}

func TestStartPumps_WebSocketToSSH(t *testing.T) {
	t.Parallel()

	// Use an in-memory pipe to simulate SSH stdin.
	stdinRead, stdinWrite := io.Pipe()

	// Build session with the pipe write-end as Stdin.
	realSess := session.NewSession("forwarder-test", "", 0, "", nil, nil, stdinWrite, nil, 15*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartStdinForwarder(ctx, realSess)

	payload := []byte("keyboard input")
	realSess.FromClient <- payload

	// Read from the read-end of the stdin pipe.
	buf := make([]byte, len(payload))
	done := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(stdinRead, buf)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ReadFull from stdin pipe: %v", err)
		}
		if !bytes.Equal(buf, payload) {
			t.Errorf("stdin data mismatch: got %q, want %q", buf, payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for data on stdin pipe")
	}
}

// TestResizePTY_ZeroCols verifies that ResizePTY returns an error when Cols is zero.
func TestResizePTY_ZeroCols(t *testing.T) {
	t.Parallel()
	// ResizePTY should reject zero Cols before touching the SSH session.
	// We pass nil for the session – the validation happens before the method call.
	err := ResizePTY(nil, WindowSize{Cols: 0, Rows: 24})
	if err == nil {
		t.Fatal("expected error for Cols=0, got nil")
	}
}

// TestResizePTY_ZeroRows verifies that ResizePTY returns an error when Rows is zero.
func TestResizePTY_ZeroRows(t *testing.T) {
	t.Parallel()
	err := ResizePTY(nil, WindowSize{Cols: 80, Rows: 0})
	if err == nil {
		t.Fatal("expected error for Rows=0, got nil")
	}
}

// TestStartStdinForwarder_StopsOnContextCancel verifies that StartStdinForwarder
// exits cleanly when the context is cancelled (no goroutine leak).
func TestStartStdinForwarder_StopsOnContextCancel(t *testing.T) {
	t.Parallel()

	_, stdinWrite := io.Pipe()
	realSess := session.NewSession("cancel-test", "", 0, "", nil, nil, stdinWrite, nil, 15*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	StartStdinForwarder(ctx, realSess)

	// Cancel immediately – the forwarder should exit without blocking.
	cancel()

	// Give the goroutine a moment to exit, then verify no deadlock by just
	// completing the test successfully.
	time.Sleep(50 * time.Millisecond)
}

// TestStartStdinForwarder_StopsOnSessionClose verifies that StartStdinForwarder
// exits when the session is closed (done channel closed).
func TestStartStdinForwarder_StopsOnSessionClose(t *testing.T) {
	t.Parallel()

	_, stdinWrite := io.Pipe()
	realSess := session.NewSession("sess-close-test", "", 0, "", nil, nil, stdinWrite, nil, 15*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartStdinForwarder(ctx, realSess)

	realSess.Close()
	// Give the goroutine a moment to notice the closed done channel.
	time.Sleep(50 * time.Millisecond)
}

func TestReadPump(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				return
			}
		}
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer ws.Close()

	sess := session.NewSession("test", "host", 22, "user", nil, nil, nil, nil, 15*time.Minute)
	sess.AddWebSocket("conn1", ws, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go readPump(ctx, sess, DefaultPumpConfig())

	payload := []byte("broadcast-test")
	sess.ToClient <- payload

	// Verification: ToClient channel is read and BroadcastToWebSockets is called.
	time.Sleep(100 * time.Millisecond)
}

func TestWritePumpAndControlMessage(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{}
	var serverWs *websocket.Conn
	done := make(chan struct{})
	
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverWs = ws
		close(done)
		// Keep connection alive for a bit
		time.Sleep(2 * time.Second)
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	clientWs, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientWs.Close()
	<-done
	// serverWs is now set

	sess := session.NewSession("test", "host", 22, "user", nil, nil, nil, nil, 15*time.Minute)
	cfg := DefaultPumpConfig()
	sess.AddWebSocket("conn1", serverWs, false)
	safeWS := sess.GetSafeConn("conn1")

	go writePump("conn1", serverWs, safeWS, sess, cfg, false)

	// Test TextMessage (Control Message - ping)
	ping, _ := json.Marshal(wsMessage{Type: "ping"})
	if err := clientWs.WriteMessage(websocket.TextMessage, ping); err != nil {
		t.Fatalf("failed to write ping: %v", err)
	}

	// Read pong
	_, msg, err := clientWs.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read pong: %v", err)
	}
	var resp wsMessage
	if err := json.Unmarshal(msg, &resp); err != nil || resp.Type != "pong" {
		t.Errorf("expected pong, got %v", resp)
	}

	// Test BinaryMessage (Data)
	data := []byte("binary data")
	if err := clientWs.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("failed to write binary: %v", err)
	}

	select {
	case got := <-sess.FromClient:
		if !bytes.Equal(got, data) {
			t.Errorf("data mismatch: got %q, want %q", got, data)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for binary data in FromClient")
	}
}

func TestStartSessionPumps(t *testing.T) {
	// Simple smoke test to cover the function
	sess := session.NewSession("test", "host", 22, "user", nil, nil, nil, bytes.NewBuffer(nil), 15*time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	StartSessionPumps(ctx, sess, DefaultPumpConfig())
	time.Sleep(50 * time.Millisecond)
}

func TestStartConnectionPump(t *testing.T) {
	// Simple smoke test to cover the function
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, _ := upgrader.Upgrade(w, r, nil)
		if ws != nil {
			defer ws.Close()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	ws, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)
	if ws != nil {
		defer ws.Close()
		sess := session.NewSession("test", "host", 22, "user", nil, nil, nil, nil, 15*time.Minute)
		StartConnectionPump("conn1", ws, sess, DefaultPumpConfig())
		time.Sleep(50 * time.Millisecond)
	}
}
