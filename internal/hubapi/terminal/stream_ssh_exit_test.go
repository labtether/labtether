package terminal

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
	"golang.org/x/crypto/ssh"
)

// This exercises the actual SSH/WebSocket bridge: a shell can finish without
// another browser input frame, and both final output streams must arrive.
func TestSSHTerminalExitDrainsOutputAndClosesBrowser(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
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
	defer listener.Close()
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		config := &ssh.ServerConfig{NoClientAuth: true}
		config.AddHostKey(signer)
		peer, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			t.Errorf("SSH server handshake: %v", err)
			return
		}
		defer peer.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			channel, reqs, err := incoming.Accept()
			if err != nil {
				t.Errorf("accept shell: %v", err)
				return
			}
			for req := range reqs {
				if req.Type == "pty-req" {
					_ = req.Reply(true, nil)
					continue
				}
				if req.Type == "shell" {
					_ = req.Reply(true, nil)
					_, _ = channel.Write([]byte("final stdout\r\n"))
					_, _ = channel.Stderr().Write([]byte("final stderr\r\n"))
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
					_ = channel.Close()
					break
				}
				_ = req.Reply(false, nil)
			}
		}
	}()
	deps := &Deps{TerminalWebSocketUpgrader: websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" },
	}}
	session := terminalmodel.Session{
		ID: "ssh-exit", Target: "test-node", InlineSSHConfig: &terminalmodel.SSHConfig{
			Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port,
			User: "test", Password: "test-fixture", StrictHostKey: true,
			HostKey: ssh.FingerprintSHA256(signer.PublicKey()),
		},
	}
	handlerDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		deps.HandleSSHTerminalStream(w, r, session)
	}))
	defer server.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	var output strings.Builder
	for {
		kind, payload, err := client.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				t.Fatalf("shell exit did not close browser normally: %v; output=%q", err, output.String())
			}
			break
		}
		if kind == websocket.BinaryMessage {
			output.Write(payload)
		}
	}
	if !strings.Contains(output.String(), "final stdout\r\n") || !strings.Contains(output.String(), "final stderr\r\n") {
		t.Fatalf("final shell output lost: %q", output.String())
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("SSH handler retained an exited session")
	}
	<-peerDone
}
