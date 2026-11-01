package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/protocols"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

// An SSH entry saved in Connect without a host key failed strict checking
// with "no SSH_HOST_KEY provided", although the Hub held the key the agent
// reported for that device. It must use that key for the same address only,
// and say plainly what is missing otherwise.
func TestConnectSSHEntryUsesAgentPinnedHostKeyForSameAddress(t *testing.T) {
	t.Setenv(shared.EnvSSHKnownHostsPath, "")
	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "")
	deviceEd25519, deviceECDSA, otherHost := testutil.SSHHostSigner(t, "ed25519"), testutil.SSHHostSigner(t, "ecdsa"), testutil.SSHHostSigner(t, "ed25519")

	manager := testutil.TestSecretsManager(t)
	store := persistence.NewMemoryCredentialStore()
	profile, err := store.CreateCredentialProfile(credentials.Profile{ID: "pw", Name: "password", Kind: credentials.KindSSHPassword, Username: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := manager.EncryptString("secret", profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateCredentialProfileSecret(profile.ID, ciphertext, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveAssetTerminalConfig(credentials.AssetTerminalConfig{
		AssetID: "srv1", Host: "192.0.2.10", Port: 22, Username: "operator", StrictHostKey: true, HostKey: testutil.AuthorizedHostKey(deviceEd25519),
	}); err != nil {
		t.Fatal(err)
	}
	entryHost := "192.0.2.10"
	d := &Deps{
		CredentialStore:        store,
		SecretsManager:         manager,
		GetAssetTerminalConfig: store.GetAssetTerminalConfig,
		GetProtocolConfig: func(context.Context, string, string) (*protocols.ProtocolConfig, error) {
			return &protocols.ProtocolConfig{AssetID: "srv1", Protocol: protocols.ProtocolSSH, Host: entryHost, Port: 22,
				Username: "operator", CredentialProfileID: profile.ID, Config: []byte(`{"strict_host_key":true}`)}, nil
		},
	}
	session := terminalmodel.Session{ID: "sess1", Target: "srv1"}

	resolved, err := d.ResolveSessionSSHConfig(session)
	if err != nil {
		t.Fatalf("resolve Connect entry: %v", err)
	}
	clientConfig, err := d.CreateSSHClientConfig(resolved, 5*time.Second)
	if err != nil {
		t.Fatalf("client config: %v", err)
	}
	if err := testutil.SSHHandshake(t, clientConfig, deviceECDSA, deviceEd25519); err != nil {
		t.Fatalf("the device's own SSH server was refused: %v", err)
	}
	if err := testutil.SSHHandshake(t, clientConfig, otherHost); err == nil {
		t.Fatal("a server with a different host key was accepted")
	}

	entryHost = "192.0.2.11"
	_, err = d.ResolveSessionSSHConfig(session)
	if !errors.Is(err, errSavedSSHHostKeyMissing) || !strings.Contains(err.Error(), "192.0.2.11") || !strings.Contains(err.Error(), "Host key field") {
		t.Fatalf("entry for another address: err=%v, want the clear missing host key message", err)
	}
}
