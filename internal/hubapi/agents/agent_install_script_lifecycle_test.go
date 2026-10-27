package agents

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// The agent trusts its pinned CA file. Adding the hub CA to the system trust
// store lets a leaked hub CA key sign certificates for any site on the host.
func TestBootstrapScriptKeepsHubCAOutOfSystemTrust(t *testing.T) {
	expectedFingerprint := strings.Repeat("b", 64)
	root, env := newAgentScriptHarness(t, expectedFingerprint)
	debianAnchor := filepath.Join(root, "usr/local/share/ca-certificates/labtether-ca.crt")
	rhelAnchor := filepath.Join(root, "etc/pki/ca-trust/source/anchors/labtether-ca.crt")
	// An older bootstrap left this copy; it matches the CA the fake hub serves.
	if err := os.WriteFile(debianAnchor, []byte("fake-ca\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := rewriteAgentScriptForHarness(GenerateBootstrapScript("https://hub.example.com", expectedFingerprint), root)
	if output, err := runGeneratedShellScript(t, script, env); err != nil {
		t.Fatalf("run bootstrap script: %v\n%s", err, output)
	}

	if _, err := os.Stat(filepath.Join(root, "etc/labtether/ca.crt")); err != nil {
		t.Fatalf("expected the pinned CA file: %v", err)
	}
	for _, anchor := range []string{debianAnchor, rhelAnchor} {
		if _, err := os.Stat(anchor); !os.IsNotExist(err) {
			t.Fatalf("hub CA left in the system trust store at %s, err=%v", anchor, err)
		}
	}
	if !strings.Contains(readFileIfExists(filepath.Join(root, "logs/ca-tools.log")), "update-ca-certificates") {
		t.Fatal("expected the trust store to be refreshed after removing the old copy")
	}
}

// The agent upgrades ws:// to wss:// and blocks loopback unless opted in, so
// without these lines an agent installed from a plain-HTTP or localhost Hub
// URL never connects. The agent also keeps any setting written to agent.env
// and ignores the Hub for it, so settings not chosen here must stay out.
func TestInstallScriptDefaultInstallForPlainHTTPLoopbackHub(t *testing.T) {
	root, env := newAgentScriptHarness(t, strings.Repeat("a", 64))
	script := rewriteAgentScriptForHarness(GenerateInstallScript("http://localhost:8080", "ws://localhost:8080/ws/agent"), root)
	if output, err := runGeneratedShellScript(t, script, env, "--skip-vnc-prereqs"); err != nil {
		t.Fatalf("run install script: %v\n%s", err, output)
	}

	envFile := mustReadFile(t, filepath.Join(root, "etc/labtether/agent.env"))
	for _, want := range []string{
		"LABTETHER_ALLOW_INSECURE_TRANSPORT=true",
		"LABTETHER_OUTBOUND_ALLOW_PRIVATE=true",
		"LABTETHER_OUTBOUND_ALLOW_LOOPBACK=true",
	} {
		if !strings.Contains(envFile, want) {
			t.Fatalf("expected env file to contain %q, got:\n%s", want, envFile)
		}
	}
	for _, unchosen := range []string{
		"LABTETHER_DOCKER_ENABLED=",
		"LABTETHER_DOCKER_SOCKET=",
		"LABTETHER_DOCKER_DISCOVERY_INTERVAL=",
		"LABTETHER_FILES_ROOT_MODE=",
	} {
		if strings.Contains(envFile, unchosen) {
			t.Fatalf("env file sets %q though no flag chose it:\n%s", unchosen, envFile)
		}
	}
}

// The Hub's SSH key gives root access. Uninstall must remove exactly the line
// the agent added, through both the installer and the CLI helper.
func TestUninstallRemovesOnlyTheHubSSHKey(t *testing.T) {
	hubKey := testAuthorizedKey(t)
	const otherKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOtherOperatorKey user@example.com"
	seedKeys := func(t *testing.T, root string) string {
		t.Helper()
		path := filepath.Join(root, "root/.ssh/authorized_keys")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(otherKey+"\n"+hubKey+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	assertOnlyHubKeyRemoved := func(t *testing.T, path string) {
		t.Helper()
		got := mustReadFile(t, path)
		if strings.Contains(got, hubKey) || !strings.Contains(got, otherKey) {
			t.Fatalf("authorized_keys after uninstall:\n%s", got)
		}
	}

	t.Run("installer uninstall", func(t *testing.T) {
		root, env := newAgentScriptHarness(t, strings.Repeat("e", 64))
		seedInstalledAgentState(t, root)
		keys := seedKeys(t, root)
		script := rewriteAgentScriptForHarness(GenerateInstallScriptWithHubSSHKey("https://hub.example.com", "wss://hub.example.com/ws/agent", hubKey+"\n"), root)
		if output, err := runGeneratedShellScript(t, script, env, "--uninstall"); err != nil {
			t.Fatalf("uninstall: %v\n%s", err, output)
		}
		assertOnlyHubKeyRemoved(t, keys)
	})

	t.Run("cli helper uninstall", func(t *testing.T) {
		root, env := newAgentScriptHarness(t, strings.Repeat("e", 64))
		script := rewriteAgentScriptForHarness(GenerateInstallScriptWithHubSSHKey("https://hub.example.com", "wss://hub.example.com/ws/agent", hubKey), root)
		if output, err := runGeneratedShellScript(t, script, env, "--skip-vnc-prereqs"); err != nil {
			t.Fatalf("install: %v\n%s", err, output)
		}
		keys := seedKeys(t, root)
		helper := mustReadFile(t, filepath.Join(root, "usr/local/bin/labtether"))
		if output, err := runGeneratedShellScript(t, helper, env, "agent", "uninstall"); err != nil {
			t.Fatalf("helper uninstall: %v\n%s", err, output)
		}
		assertOnlyHubKeyRemoved(t, keys)
	})
}

func testAuthorizedKey(t *testing.T) string {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}
