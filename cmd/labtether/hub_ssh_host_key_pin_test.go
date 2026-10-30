package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// The saved Hub-key login could never connect: strict host-key checking with
// no host key. The host keys an authenticated agent reports must be pinned,
// the client must ask for the pinned key's type (Go prefers ECDSA, so a
// pinned ed25519 key failed against a server with both), and a server
// presenting any other key must be refused.
func TestInstallReplyHostKeysPinTheSSHFallback(t *testing.T) {
	t.Setenv(shared.EnvSSHKnownHostsPath, "")
	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "")
	server := newTestAPIServer(t)
	initializeTestHubSSHIdentity(t, server)
	enableTestHubSSHKey(t, server, "node-pin")

	deviceEd25519, deviceECDSA, otherHost := testSSHHostSigner(t, "ed25519"), testSSHHostSigner(t, "ecdsa"), testSSHHostSigner(t, "ed25519")
	authorized := func(signer ssh.Signer) string {
		return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	}
	conn := &agentmgr.AgentConn{AssetID: "node-pin"}
	conn.SetMeta("ssh_source_ip", "192.0.2.10")
	server.processAgentSSHKeyInstalled(conn, marshalAgentMessage(t, agentmgr.SSHKeyInstalledData{
		Username: "labuser",
		HostKeys: []string{authorized(deviceECDSA), "not a key", authorized(deviceEd25519)},
	}))

	deps := server.ensureTerminalDeps()
	resolved, err := deps.ResolveSessionSSHConfig(terminal.Session{ID: "sess-pin", Target: "node-pin"})
	if err != nil || resolved == nil {
		t.Fatalf("resolve saved login: %v", err)
	}
	if resolved.HostKey != authorized(deviceEd25519) {
		t.Fatalf("pinned host key=%q, want the device's ed25519 key", resolved.HostKey)
	}
	clientConfig, err := deps.CreateSSHClientConfig(resolved, 5*time.Second)
	if err != nil {
		t.Fatalf("client config: %v", err)
	}
	if err := testSSHHandshake(t, clientConfig, deviceECDSA, deviceEd25519); err != nil {
		t.Fatalf("the device's own SSH server was refused: %v", err)
	}
	if err := testSSHHandshake(t, clientConfig, otherHost); err == nil {
		t.Fatal("a server with a different host key was accepted")
	}
}

func testSSHHostSigner(t *testing.T, kind string) ssh.Signer {
	t.Helper()
	var private any
	var err error
	if kind == "ecdsa" {
		private, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	} else {
		_, private, err = ed25519.GenerateKey(rand.Reader)
	}
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

// testSSHHandshake connects to a local SSH server holding hostKeys and
// returns the client's handshake error.
func testSSHHandshake(t *testing.T, clientConfig *ssh.ClientConfig, hostKeys ...ssh.Signer) error {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	for _, key := range hostKeys {
		serverConfig.AddHostKey(key)
	}
	go func() {
		accepted, err := listener.Accept()
		if err != nil {
			return
		}
		defer accepted.Close()
		if _, channels, requests, err := ssh.NewServerConn(accepted, serverConfig); err == nil {
			go ssh.DiscardRequests(requests)
			for channel := range channels {
				_ = channel.Reject(ssh.Prohibited, "test server")
			}
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), clientConfig)
	if client != nil {
		_ = client.Close()
	}
	return err
}
