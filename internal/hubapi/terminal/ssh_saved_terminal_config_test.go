package terminal

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

// With the agent stopped, a terminal must fall back to the SSH login the Hub
// saved when the agent installed the Hub key. It was never read, so the
// session ended with "no SSH credentials configured".
func TestResolveSessionSSHConfigUsesSavedHubKeyLogin(t *testing.T) {
	t.Setenv(shared.EnvSSHKnownHostsPath, "")
	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "")
	const hostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIC8k2qDwE65vXWA6WUMUU7UiqzXUiFDX7xWszP1A0C/W"
	manager, err := secrets.NewManagerFromEncodedKey(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	store := persistence.NewMemoryCredentialStore()
	profile, err := store.CreateCredentialProfile(credentials.Profile{ID: "hub-key", Name: "Hub SSH key", Kind: credentials.KindHubSSHIdentity})
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := manager.EncryptString("private-key-pem", profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateCredentialProfileSecret(profile.ID, ciphertext, "", nil); err != nil {
		t.Fatal(err)
	}
	keyOn := true
	d := &Deps{
		CredentialStore:        store,
		SecretsManager:         manager,
		GetAssetTerminalConfig: store.GetAssetTerminalConfig,
		HubSSHKeyEnabled:       func(string) bool { return keyOn },
	}
	save := func(user, key string) {
		if _, err := store.SaveAssetTerminalConfig(credentials.AssetTerminalConfig{
			AssetID: "srv1", Host: "192.0.2.10", Port: 22, Username: user, StrictHostKey: true, HostKey: key, CredentialProfileID: profile.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	session := terminalmodel.Session{ID: "sess1", Target: "srv1"}

	save("operator", hostKey)
	resolved, err := d.ResolveSessionSSHConfig(session)
	if err != nil || resolved == nil {
		t.Fatalf("saved login not used: %v", err)
	}
	if resolved.Host != "192.0.2.10" || resolved.User != "operator" || resolved.PrivateKey != "private-key-pem" || !resolved.StrictHostKey {
		t.Fatalf("resolved = %+v", resolved)
	}

	save(`WORKGROUP\HOST$`, hostKey)
	if _, err := d.ResolveSessionSSHConfig(session); !errors.Is(err, errSavedSSHUserMissing) {
		t.Fatalf("machine account as SSH user: err=%v, want the missing-user message", err)
	}

	// An older agent sent no host key: say what to do instead of failing on
	// strict host-key checking later.
	save("operator", "")
	if _, err := d.ResolveSessionSSHConfig(session); !errors.Is(err, errSavedSSHHostKeyMissing) {
		t.Fatalf("no host key: err=%v, want the missing host key message", err)
	}

	save("operator", hostKey)
	keyOn = false
	if _, err := d.ResolveSessionSSHConfig(session); !errors.Is(err, shared.ErrHubSSHKeyOff) {
		t.Fatalf("Hub key off: err=%v", err)
	}
}
