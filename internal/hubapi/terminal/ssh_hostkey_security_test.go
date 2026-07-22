package terminal

import (
	"testing"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

func TestAssetTerminalConfigDefaultsToStrictHostKeyVerification(t *testing.T) {
	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "")
	d := &Deps{}

	resolved, err := d.ResolveAssetTerminalConfig(credentialsConfigForHostKeyTest(false))
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if !resolved.StrictHostKey {
		t.Fatal("zero-value per-asset setting must not disable SSH host-key verification")
	}
}

func TestBuildSSHHostKeyCallbackRequiresGlobalInsecureAcknowledgement(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_KNOWN_HOSTS_PATH", "")
	t.Setenv("SSH_KNOWN_HOSTS_PATHS", "")
	d := &Deps{}
	cfg := &terminalmodel.SSHConfig{StrictHostKey: false}

	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "")
	if _, err := d.buildSSHHostKeyCallback(cfg); err == nil {
		t.Fatal("expected fail-closed host-key error without explicit acknowledgement")
	}

	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "true")
	if callback, err := d.buildSSHHostKeyCallback(cfg); err != nil || callback == nil {
		t.Fatalf("expected explicitly acknowledged insecure callback, callback=%v err=%v", callback, err)
	}
}

func credentialsConfigForHostKeyTest(strict bool) credentials.AssetTerminalConfig {
	return credentials.AssetTerminalConfig{
		AssetID:       "srv1",
		Host:          "192.0.2.10",
		Port:          22,
		Username:      "operator",
		StrictHostKey: strict,
	}
}
