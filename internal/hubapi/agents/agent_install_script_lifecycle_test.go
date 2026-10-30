package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The agent trusts its pinned CA file. Adding the hub CA to the system trust
// store lets a leaked hub CA key sign certificates for any site on the host,
// and a copy from before a hub CA reset must not stay trusted either.
func TestBootstrapScriptKeepsHubCAOutOfSystemTrust(t *testing.T) {
	expectedFingerprint := strings.Repeat("b", 64)
	run := func(t *testing.T, debian, rhel string) (string, string, string, string) {
		t.Helper()
		root, env := newAgentScriptHarness(t, expectedFingerprint)
		debianAnchor := filepath.Join(root, "usr/local/share/ca-certificates/labtether-ca.crt")
		rhelAnchor := filepath.Join(root, "etc/pki/ca-trust/source/anchors/labtether-ca.crt")
		for path, contents := range map[string]string{debianAnchor: debian, rhelAnchor: rhel} {
			if contents == "" {
				continue
			}
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		script := rewriteAgentScriptForHarness(GenerateBootstrapScript("https://hub.example.com", expectedFingerprint), root)
		output, err := runGeneratedShellScript(t, script, env)
		if err != nil {
			t.Fatalf("run bootstrap script: %v\n%s", err, output)
		}
		if _, err := os.Stat(filepath.Join(root, "etc/labtether/ca.crt")); err != nil {
			t.Fatalf("expected the pinned CA file: %v", err)
		}
		return root, output, debianAnchor, rhelAnchor
	}

	t.Run("removes current and pre-reset copies", func(t *testing.T) {
		// The fake openssl prints the file, so this reads as a LabTether CA
		// that no longer matches the hub's current CA.
		stale := "subject=CN = LabTether CA, O = LabTether\nCA:TRUE\n"
		root, _, debianAnchor, rhelAnchor := run(t, "fake-ca\n", stale)
		for _, anchor := range []string{debianAnchor, rhelAnchor} {
			if _, err := os.Stat(anchor); !os.IsNotExist(err) {
				t.Fatalf("hub CA left in the system trust store at %s, err=%v", anchor, err)
			}
		}
		if !strings.Contains(readFileIfExists(filepath.Join(root, "logs/ca-tools.log")), "update-ca-certificates") {
			t.Fatal("expected the trust store to be refreshed after removing the old copies")
		}
	})

	t.Run("keeps a file that is not a LabTether CA", func(t *testing.T) {
		_, output, debianAnchor, _ := run(t, "someone else's certificate\n", "")
		if got := readFileIfExists(debianAnchor); got != "someone else's certificate\n" {
			t.Fatalf("foreign trust anchor changed: %q", got)
		}
		if !strings.Contains(output, "Kept ") {
			t.Fatalf("expected a warning for the kept file, got:\n%s", output)
		}
	})
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

// The Hub's SSH key gives root access. Uninstall must remove every key line
// the Hub added, including one from before a Hub key rotation, through both
// the installer and the CLI helper, and leave every other key alone.
func TestUninstallRemovesOnlyHubSSHKeys(t *testing.T) {
	const (
		userKey        = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOtherOperatorKey user@example.com"
		userCopyOfHub  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHubKeyNoComment"
		hubKeyCurrent  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHubKeyCurrent labtether-hub"
		hubKeyRotated  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHubKeyRotated labtether-hub"
		userKeysOnly   = userKey + "\n" + userCopyOfHub + "\n"
		seededKeysFile = userKey + "\n" + hubKeyRotated + "\n" + userCopyOfHub + "\n" + hubKeyCurrent + "\n"
	)
	seedKeys := func(t *testing.T, root string) string {
		t.Helper()
		path := filepath.Join(root, "root/.ssh/authorized_keys")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(seededKeysFile), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	script := func(root string) string {
		return rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
	}

	t.Run("installer uninstall", func(t *testing.T) {
		root, env := newAgentScriptHarness(t, strings.Repeat("e", 64))
		seedInstalledAgentState(t, root)
		keys := seedKeys(t, root)
		if output, err := runGeneratedShellScript(t, script(root), env, "--uninstall"); err != nil {
			t.Fatalf("uninstall: %v\n%s", err, output)
		}
		if got := mustReadFile(t, keys); got != userKeysOnly {
			t.Fatalf("authorized_keys after uninstall:\n%s", got)
		}
	})

	t.Run("cli helper uninstall", func(t *testing.T) {
		root, env := newAgentScriptHarness(t, strings.Repeat("e", 64))
		if output, err := runGeneratedShellScript(t, script(root), env, "--skip-vnc-prereqs"); err != nil {
			t.Fatalf("install: %v\n%s", err, output)
		}
		keys := seedKeys(t, root)
		helper := mustReadFile(t, filepath.Join(root, "usr/local/bin/labtether"))
		if output, err := runGeneratedShellScript(t, helper, env, "agent", "uninstall"); err != nil {
			t.Fatalf("helper uninstall: %v\n%s", err, output)
		}
		if got := mustReadFile(t, keys); got != userKeysOnly {
			t.Fatalf("authorized_keys after helper uninstall:\n%s", got)
		}
	})

	t.Run("unwritable key folder does not stop uninstall", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can write to a read-only folder")
		}
		root, env := newAgentScriptHarness(t, strings.Repeat("e", 64))
		seedInstalledAgentState(t, root)
		keys := seedKeys(t, root)
		if err := os.Chmod(filepath.Dir(keys), 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Dir(keys), 0o700) })
		output, err := runGeneratedShellScript(t, script(root), env, "--uninstall")
		if err != nil || !strings.Contains(output, "Could not update") {
			t.Fatalf("uninstall err=%v, want a warning and success:\n%s", err, output)
		}
		for _, path := range []string{"usr/local/bin/labtether-agent", "etc/labtether/agent.env"} {
			if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
				t.Fatalf("uninstall stopped before removing %s, err=%v", path, err)
			}
		}
	})
}
