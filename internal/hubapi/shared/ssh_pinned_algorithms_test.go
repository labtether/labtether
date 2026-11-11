package shared_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"net"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/protocols"
)

// A device's sshd usually has an ECDSA and an ed25519 host key. With the
// ed25519 key pinned (the line people copy from ssh_host_ed25519_key.pub),
// Test connection must log in, not fail with "host key mismatch" because the
// server presented its ECDSA key.
func TestSSHTestConnectionWithPinnedEd25519KeyOnServerWithECDSA(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	_, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edSigner, err := ssh.NewSignerFromKey(edPriv)
	if err != nil {
		t.Fatal(err)
	}
	ecSigner, err := ssh.NewSignerFromKey(ecPriv)
	if err != nil {
		t.Fatal(err)
	}
	server := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	server.AddHostKey(ecSigner)
	server.AddHostKey(edSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer conn.Close()
				if _, chans, reqs, handshakeErr := ssh.NewServerConn(conn, server); handshakeErr == nil {
					go ssh.DiscardRequests(reqs)
					for ch := range chans {
						_ = ch.Reject(ssh.Prohibited, "test server")
					}
				}
			}()
		}
	}()
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	pinned := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(edSigner.PublicKey())))

	callback, err := shared.BuildSSHHostKeyCallback(true, pinned)
	if err != nil {
		t.Fatal(err)
	}
	algorithms := shared.PinnedHostKeyAlgorithms(true, pinned)
	if len(algorithms) != 1 || algorithms[0] != ssh.KeyAlgoED25519 {
		t.Fatalf("algorithms for a pinned ed25519 key = %v", algorithms)
	}
	if result := protocols.TestSSH(context.Background(), host, port, "ltqa", "secret", "", callback, algorithms); !result.Success {
		t.Fatalf("Test connection with the pinned ed25519 key: %#v", result)
	}
	// Without the limit the server presents its ECDSA key and the pin does not match.
	if result := protocols.TestSSH(context.Background(), host, port, "ltqa", "secret", "", callback, nil); result.Success || !strings.Contains(result.Error, "host key mismatch") {
		t.Fatalf("without pinned algorithms the test should show the old mismatch, got %#v", result)
	}
}
