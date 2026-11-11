package terminal

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/terminal"
)

// startPasswordSSHServer runs an SSH server with one host key that notes
// whether any login reached it.
func startPasswordSSHServer(t *testing.T, signer ssh.Signer, loginSeen *atomic.Bool) (string, int) {
	t.Helper()
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
		loginSeen.Store(true)
		return nil, nil
	}}
	cfg.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				if _, chans, reqs, err := ssh.NewServerConn(conn, cfg); err == nil {
					go ssh.DiscardRequests(reqs)
					for ch := range chans {
						_ = ch.Reject(ssh.Prohibited, "no channels")
					}
				}
			}()
		}
	}()
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	return host, port
}

func newHostSigner(t *testing.T) (ssh.Signer, string) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
}

// A server that shows another key than the one trusted (for example one that
// swapped keys after the check) is refused during key exchange: no password
// reaches it, and the terminal names both fingerprints.
func TestPinnedHostKeyRefusesAnotherKeyBeforeLogin(t *testing.T) {
	signerA, keyA := newHostSigner(t)
	signerB, _ := newHostSigner(t)
	var loginSeen atomic.Bool
	host, port := startPasswordSSHServer(t, signerB, &loginSeen)
	resolved := &terminal.SSHConfig{Host: host, Port: port, User: "demo", Password: "pw-not-sent", StrictHostKey: true, HostKey: keyA}
	d := &Deps{}

	cfg, err := d.CreateSSHClientConfig(resolved, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	watch := watchHostKey(cfg, resolved)
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	if client, err := ssh.Dial("tcp", addr, cfg); err == nil {
		_ = client.Close()
		t.Fatal("connected to a server with another host key")
	}
	if loginSeen.Load() {
		t.Fatal("the password reached a server whose host key did not match the pin")
	}
	message, changed := watch.changedMessage(resolved)
	if !changed || !strings.Contains(message, ssh.FingerprintSHA256(signerA.PublicKey())) ||
		!strings.Contains(message, ssh.FingerprintSHA256(signerB.PublicKey())) {
		t.Fatalf("changed-key message: %q", message)
	}

	resolved.HostKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signerB.PublicKey())))
	cfg, _ = d.CreateSSHClientConfig(resolved, 5*time.Second)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		t.Fatalf("connect with the matching pin: %v", err)
	}
	_ = client.Close()
	if !loginSeen.Load() {
		t.Fatal("the login should reach the server once its key matches the pin")
	}
}
