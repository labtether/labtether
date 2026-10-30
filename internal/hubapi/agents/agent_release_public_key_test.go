package agents

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Auto-update was enabled by the installer but could never succeed: agents
// refuse unsigned updates and the installer never wrote the trusted key.
func TestInstallScriptWritesReleaseSigningKeyForAutoUpdate(t *testing.T) {
	key := AgentReleasePublicKey()
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		t.Fatalf("embedded release key is not a base64 Ed25519 public key: len=%d err=%v", len(decoded), err)
	}
	script := GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent")
	for _, want := range []string{
		"AUTO_UPDATE_TRUSTED_PUBLIC_KEY='" + key + "'",
		"LABTETHER_AUTO_UPDATE_TRUSTED_PUBLIC_KEY=${AUTO_UPDATE_TRUSTED_PUBLIC_KEY}",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("install script is missing %q", want)
		}
	}
	if strings.Contains(script, "%!") {
		t.Fatal("install script template has a formatting error")
	}
}

// The embedded copy must stay identical to the release tooling's key.
func TestEmbeddedReleaseKeyMatchesReleaseScripts(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "release", "agent-release-public-key.b64"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := AgentReleasePublicKey(), strings.TrimSpace(string(raw)); got != want {
		t.Fatalf("embedded agent release key %q differs from scripts/release copy %q", got, want)
	}
	// The Windows service helper sets the same key, or its agents refuse
	// every self-update.
	windowsScript, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "agents", "windows", "install-agent.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(windowsScript), `$UpdatePublicKey  = "`+AgentReleasePublicKey()+`"`) {
		t.Fatal("deploy/agents/windows/install-agent.ps1 does not default to the agent release key")
	}
}
