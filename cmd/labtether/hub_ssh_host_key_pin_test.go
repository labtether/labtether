package main

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
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

	deviceEd25519, deviceECDSA, otherHost := testutil.SSHHostSigner(t, "ed25519"), testutil.SSHHostSigner(t, "ecdsa"), testutil.SSHHostSigner(t, "ed25519")
	authorized := testutil.AuthorizedHostKey
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
	if err := testutil.SSHHandshake(t, clientConfig, deviceECDSA, deviceEd25519); err != nil {
		t.Fatalf("the device's own SSH server was refused: %v", err)
	}
	if err := testutil.SSHHandshake(t, clientConfig, otherHost); err == nil {
		t.Fatal("a server with a different host key was accepted")
	}
}
