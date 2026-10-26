package terminal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// The jump host completes SSH and accepts forwarding, then stops reading.
// A channel-close packet cannot wake the target's silent SSH handshake.
func TestSSHStalledJumpHandshakeCancellationClosesRoot(t *testing.T) {
	root, peer, config := startStallingSSHPeer(t)
	tunnel, err := root.Dial("tcp", "test-target:22")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := handshakeSSHThroughChain(ctx, tunnel, "test-target:22", config, root)
		finished <- err
	}()
	select {
	case <-peer.blocked:
	case <-time.After(time.Second):
		t.Fatal("jump host did not enter stalled read")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("handshake error=%v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled handshake stayed blocked on the jump channel")
	}
}

func TestSSHStalledShellStartupUsesOwnedChainCleanup(t *testing.T) {
	root, _, _ := startStallingSSHPeer(t)
	var closed atomic.Bool
	finished := make(chan error, 1)
	go func() {
		_, _, _, _, err := startSSHInteractiveSessionWithCleanup(root, 80, 24, "", 20*time.Millisecond, func() error {
			closed.Store(true)
			return root.Close()
		})
		finished <- err
	}()
	select {
	case err := <-finished:
		if !closed.Load() || err == nil || !strings.Contains(err.Error(), "startup timed out") {
			t.Fatalf("startup skipped owned-chain cleanup: closed=%v err=%v", closed.Load(), err)
		}
	case <-time.After(time.Second):
		t.Fatal("shell startup timeout stayed blocked")
	}
}

func TestSSHStalledScrollbackCaptureClosesOwnedChain(t *testing.T) {
	root, _, _ := startStallingSSHPeer(t)
	var closed atomic.Bool
	finished := make(chan error, 1)
	go func() {
		_, err := captureSSHScrollback(root, "fixture capture", func() error {
			closed.Store(true)
			return root.Close()
		}, 20*time.Millisecond)
		finished <- err
	}()
	select {
	case err := <-finished:
		if !closed.Load() || !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("capture skipped owned-chain timeout: closed=%v err=%v", closed.Load(), err)
		}
	case <-time.After(time.Second):
		t.Fatal("scrollback capture stayed blocked")
	}
}

func TestSSHScrollbackOutputKeepsBoundedTail(t *testing.T) {
	output := &sshReplayOutput{limit: 8}
	for _, payload := range []string{"123456", "abcdef", "most recent output"} {
		if n, err := output.Write([]byte(payload)); err != nil || n != len(payload) {
			t.Fatalf("noisy output stopped draining: n=%d err=%v", n, err)
		}
		if len(output.bytes()) > 8 {
			t.Fatal("capture exceeded its memory bound")
		}
	}
	if got := string(output.bytes()); got != "t output" {
		t.Fatalf("capture lost newest output: %q", got)
	}
}

func startStallingSSHPeer(t *testing.T) (*ssh.Client, *stalledJumpReadConn, *ssh.ClientConfig) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	peerConn := make(chan *stalledJumpReadConn, 1)
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		stalled := &stalledJumpReadConn{Conn: conn, closed: make(chan struct{}), blocked: make(chan struct{})}
		peerConn <- stalled
		defer stalled.Close()
		config := &ssh.ServerConfig{NoClientAuth: true}
		config.AddHostKey(signer)
		peer, channels, requests, err := ssh.NewServerConn(stalled, config)
		if err != nil {
			return
		}
		defer peer.Close()
		go ssh.DiscardRequests(requests)
		incoming, ok := <-channels
		if !ok {
			return
		}
		stalled.pause.Store(true)
		_, channelRequests, err := incoming.Accept()
		if err != nil {
			return
		}
		go func() {
			for range channelRequests {
			}
		}()
		_ = peer.Wait()
	}()
	config := &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: time.Second}
	root, err := ssh.Dial("tcp", listener.Addr().String(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	peer := <-peerConn
	t.Cleanup(func() {
		_ = peer.Close()
		<-peerDone
	})
	return root, peer, config
}

type stalledJumpReadConn struct {
	net.Conn
	pause       atomic.Bool
	closed      chan struct{}
	blocked     chan struct{}
	closeOnce   sync.Once
	blockedOnce sync.Once
}

func (conn *stalledJumpReadConn) Read(data []byte) (int, error) {
	if conn.pause.Load() {
		conn.blockedOnce.Do(func() { close(conn.blocked) })
		<-conn.closed
		return 0, net.ErrClosed
	}
	return conn.Conn.Read(data)
}

func (conn *stalledJumpReadConn) Close() error {
	conn.closeOnce.Do(func() { close(conn.closed) })
	return conn.Conn.Close()
}
