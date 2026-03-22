package agents

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestInstallScriptExecutesInstallFlow(t *testing.T) {
	root, env := newAgentScriptHarness(t, strings.Repeat("a", 64))

	caPath := filepath.Join(root, "fixtures", "hub-ca.crt")
	if err := os.MkdirAll(filepath.Dir(caPath), 0o755); err != nil {
		t.Fatalf("mkdir fixtures: %v", err)
	}
	if err := os.WriteFile(caPath, []byte("test-ca"), 0o644); err != nil {
		t.Fatalf("write CA file: %v", err)
	}

	script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
	_, err := runGeneratedShellScript(t, script, env,
		"--docker-enabled", "true",
		"--docker-endpoint", "/var/run/docker.sock",
		"--docker-discovery-interval", "45",
		"--files-root-mode", "full",
		"--auto-update", "false",
		"--skip-vnc-prereqs",
		"--enrollment-token", "enroll-123",
		"--tls-ca-file", caPath,
	)
	if err != nil {
		t.Fatalf("run install script: %v", err)
	}

	envFile := mustReadFile(t, filepath.Join(root, "etc/labtether/agent.env"))
	for _, want := range []string{
		"LABTETHER_WS_URL=wss://hub.example.com/ws/agent",
		"LABTETHER_ENROLLMENT_TOKEN=enroll-123",
		"LABTETHER_DOCKER_ENABLED=true",
		`LABTETHER_DOCKER_SOCKET="/var/run/docker.sock"`,
		"LABTETHER_DOCKER_DISCOVERY_INTERVAL=45",
		"LABTETHER_FILES_ROOT_MODE=full",
		"LABTETHER_AUTO_UPDATE=false",
		"LABTETHER_TLS_CA_FILE=" + caPath,
	} {
		if !strings.Contains(envFile, want) {
			t.Fatalf("expected env file to contain %q, got:\n%s", want, envFile)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "usr/local/bin/labtether-agent")); err != nil {
		t.Fatalf("expected installed agent binary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "usr/local/bin/labtether")); err != nil {
		t.Fatalf("expected installed CLI helper: %v", err)
	}

	systemctlLog := mustReadFile(t, filepath.Join(root, "logs/systemctl.log"))
	for _, want := range []string{"daemon-reload", "enable --now labtether-agent"} {
		if !strings.Contains(systemctlLog, want) {
			t.Fatalf("expected systemctl log to contain %q, got:\n%s", want, systemctlLog)
		}
	}

	agentBinaryLog := mustReadFile(t, filepath.Join(root, "logs/agent-binary.log"))
	if !strings.Contains(agentBinaryLog, "settings test docker /var/run/docker.sock") {
		t.Fatalf("expected installed binary to receive docker settings test, got:\n%s", agentBinaryLog)
	}
}

func TestBootstrapScriptExecutesPinnedInstallFlow(t *testing.T) {
	expectedFingerprint := strings.Repeat("b", 64)
	root, env := newAgentScriptHarness(t, expectedFingerprint)

	script := rewriteAgentScriptForHarness(GenerateBootstrapScript("https://hub.example.com", expectedFingerprint), root)
	output, err := runGeneratedShellScript(t, script, env, "--enrollment-token", "bootstrap-token")
	if err != nil {
		t.Fatalf("run bootstrap script: %v", err)
	}

	curlLog := mustReadFile(t, filepath.Join(root, "logs/curl-urls.log"))
	installArgs := readFileIfExists(filepath.Join(root, "logs/bootstrap-install-args.txt"))
	caPath := filepath.Join(root, "etc/labtether/ca.crt")
	if _, err := os.Stat(caPath); err != nil {
		t.Fatalf("expected bootstrap CA file to be installed: %v\noutput:\n%s\ncurl log:\n%s\ninstall args:\n%s\ntree:\n%s", err, output, curlLog, installArgs, dumpTree(t, root))
	}

	for _, want := range []string{"/api/v1/ca.crt", "/install.sh"} {
		if !strings.Contains(curlLog, want) {
			t.Fatalf("expected bootstrap downloads to include %q, got:\n%s", want, curlLog)
		}
	}

	for _, want := range []string{"--tls-ca-file", caPath, "--enrollment-token", "bootstrap-token"} {
		if !strings.Contains(installArgs, want) {
			t.Fatalf("expected bootstrap-installed script args to contain %q, got:\n%s", want, installArgs)
		}
	}
}

func TestInstallScriptReinstallPreservesPersistedTokenWithoutEnrollmentToken(t *testing.T) {
	root, env := newAgentScriptHarness(t, strings.Repeat("c", 64))

	tokenPath := filepath.Join(root, "etc/labtether/agent-token")
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err != nil {
		t.Fatalf("mkdir token dir: %v", err)
	}
	if err := os.WriteFile(tokenPath, []byte("persisted-token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	caPath := filepath.Join(root, "fixtures", "hub-ca.crt")
	if err := os.MkdirAll(filepath.Dir(caPath), 0o755); err != nil {
		t.Fatalf("mkdir fixtures: %v", err)
	}
	if err := os.WriteFile(caPath, []byte("test-ca"), 0o644); err != nil {
		t.Fatalf("write CA file: %v", err)
	}

	script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
	if _, err := runGeneratedShellScript(t, script, env, "--skip-vnc-prereqs", "--tls-ca-file", caPath); err != nil {
		t.Fatalf("run reinstall script: %v", err)
	}

	if got := mustReadFile(t, tokenPath); !strings.Contains(got, "persisted-token") {
		t.Fatalf("expected persisted token to survive reinstall, got %q", got)
	}
}

func TestInstallScriptDesktopPrereqsInstallIncludesGStreamerAndInputTools(t *testing.T) {
	root, env := newAgentScriptHarness(t, strings.Repeat("f", 64))

	script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
	if output, err := runGeneratedShellScript(t, script, env, "--install-vnc-prereqs"); err != nil {
		t.Fatalf("run install script with desktop prereqs: %v\noutput:\n%s", err, output)
	}

	aptLog := mustReadFile(t, filepath.Join(root, "logs/apt-get.log"))
	for _, want := range []string{
		"update -y",
		"install -y",
		"x11vnc",
		"xvfb",
		"xterm",
		"xdotool",
		"gstreamer1.0-tools",
		"gstreamer1.0-plugins-base",
		"gstreamer1.0-plugins-good",
		"gstreamer1.0-plugins-bad",
		"gstreamer1.0-plugins-ugly",
		"gstreamer1.0-libav",
		"gstreamer1.0-x",
	} {
		if !strings.Contains(aptLog, want) {
			t.Fatalf("expected apt-get log to contain %q, got:\n%s", want, aptLog)
		}
	}
}

func TestInstallScriptSummaryWrapsLongValues(t *testing.T) {
	root, env := newAgentScriptHarness(t, strings.Repeat("9", 64))

	longFingerprint := "LT-WJHN-I4WG-VI7N-6TM6-A636-J6ZP-EAPV-WWXV-VLNW-GKZR-TU4M-JTCB-PX3A"
	longHostname := "containervm-deltaserver-with-a-very-long-hostname-for-summary-wrap-tests"

	writeExecutable(t, filepath.Join(root, "bin", "hostname"), "#!/bin/bash\nset -euo pipefail\nprintf '%s\\n' \""+longHostname+"\"\n")
	writeExecutable(t, filepath.Join(root, "bin", "systemctl"), `#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >> "${LABTETHER_TEST_LOG_DIR}/systemctl.log"
if [[ "${1:-}" == "enable" ]]; then
  mkdir -p "$(dirname "${LABTETHER_TEST_FINGERPRINT_FILE}")"
  printf '%s\n' "${LABTETHER_TEST_FINGERPRINT}" > "${LABTETHER_TEST_FINGERPRINT_FILE}"
fi
if [[ "${1:-}" == "is-active" ]]; then
  exit 1
fi
exit 0
`)
	env = append(env,
		"LABTETHER_TEST_FINGERPRINT="+longFingerprint,
		"LABTETHER_TEST_FINGERPRINT_FILE="+filepath.Join(root, "etc/labtether/device-fingerprint"),
	)

	script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
	output, err := runGeneratedShellScript(t, script, env,
		"--docker-enabled", "true",
		"--docker-endpoint", "/var/run/docker.sock",
		"--skip-vnc-prereqs",
		"--enrollment-token", "enroll-123",
	)
	if err != nil {
		t.Fatalf("run install script: %v\noutput:\n%s", err, output)
	}

	lines := strings.Split(output, "\n")
	sawSummary := false
	for _, line := range lines {
		if strings.Contains(line, "Installation Complete") {
			sawSummary = true
		}
		if strings.Contains(line, "│") {
			if !strings.HasPrefix(line, "  │") || !strings.HasSuffix(line, "│") {
				t.Fatalf("expected boxed line to stay aligned, got %q", line)
			}
			if len([]rune(line)) > 100 {
				t.Fatalf("expected wrapped summary line, got %d chars: %q", len([]rune(line)), line)
			}
		}
	}
	if !sawSummary {
		t.Fatalf("expected installation summary in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Verify this fingerprint in LabTether before approving") {
		t.Fatalf("expected fingerprint verification message, got:\n%s", output)
	}
	if !strings.Contains(output, "Auto-enrollment configured") {
		t.Fatalf("expected auto-enrollment summary message, got:\n%s", output)
	}
}

func TestInstallScriptUninstallAndPurgeLifecycle(t *testing.T) {
	t.Run("uninstall preserves identity material", func(t *testing.T) {
		root, env := newAgentScriptHarness(t, strings.Repeat("d", 64))
		seedInstalledAgentState(t, root)

		script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
		if _, err := runGeneratedShellScript(t, script, env, "--uninstall"); err != nil {
			t.Fatalf("run uninstall script: %v", err)
		}

		for _, path := range []string{
			filepath.Join(root, "usr/local/bin/labtether-agent"),
			filepath.Join(root, "etc/labtether/agent.env"),
			filepath.Join(root, "etc/systemd/system/labtether-agent.service"),
		} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("expected %s to be removed after uninstall, err=%v", path, err)
			}
		}

		for _, path := range []string{
			filepath.Join(root, "etc/labtether/agent-token"),
			filepath.Join(root, "etc/labtether/device-key"),
			filepath.Join(root, "etc/labtether/device-key.pub"),
			filepath.Join(root, "etc/labtether/device-fingerprint"),
		} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("expected %s to remain after uninstall: %v", path, err)
			}
		}
	})

	t.Run("purge removes identity material", func(t *testing.T) {
		root, env := newAgentScriptHarness(t, strings.Repeat("e", 64))
		seedInstalledAgentState(t, root)

		script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
		if _, err := runGeneratedShellScript(t, script, env, "--purge"); err != nil {
			t.Fatalf("run purge script: %v", err)
		}

		for _, path := range []string{
			filepath.Join(root, "usr/local/bin/labtether-agent"),
			filepath.Join(root, "etc/labtether/agent.env"),
			filepath.Join(root, "etc/labtether/agent-token"),
			filepath.Join(root, "etc/labtether/device-key"),
			filepath.Join(root, "etc/labtether/device-key.pub"),
			filepath.Join(root, "etc/labtether/device-fingerprint"),
			filepath.Join(root, "etc/systemd/system/labtether-agent.service"),
		} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("expected %s to be removed after purge, err=%v", path, err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "etc/labtether")); !os.IsNotExist(err) {
			t.Fatalf("expected config directory to be removed after purge, err=%v", err)
		}
	})
}

func newAgentScriptHarness(t *testing.T, expectedFingerprint string) (string, []string) {
	t.Helper()

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	logDir := filepath.Join(root, "logs")
	for _, dir := range []string{
		binDir,
		logDir,
		filepath.Join(root, "etc"),
		filepath.Join(root, "usr/local/bin"),
		filepath.Join(root, "usr/local/share/ca-certificates"),
		filepath.Join(root, "etc/systemd/system"),
		filepath.Join(root, "etc/pki/ca-trust/source/anchors"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	writeExecutable(t, filepath.Join(binDir, "curl"), `#!/bin/bash
set -euo pipefail

out=""
url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output|-o)
      out="$2"
      shift 2
      ;;
    --cacert)
      shift 2
      ;;
    --no-check-certificate)
      shift
      ;;
    -*)
      shift
      ;;
    *)
      url="$1"
      shift
      ;;
  esac
done

printf '%s\n' "${url}" >> "${LABTETHER_TEST_LOG_DIR}/curl-urls.log"

if [[ -n "${out}" ]]; then
  case "${url}" in
    *"/api/v1/agent/binary?arch="*)
      cat > "${out}" <<'BIN'
#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >> "${LABTETHER_TEST_LOG_DIR}/agent-binary.log"
exit 0
BIN
      chmod 755 "${out}"
      ;;
    *"/api/v1/ca.crt")
      printf 'fake-ca\n' > "${out}"
      ;;
    *"/install.sh")
      cat > "${out}" <<'INSTALL'
#!/bin/bash
set -euo pipefail
printf '%s\n' "$@" > "${LABTETHER_TEST_LOG_DIR}/bootstrap-install-args.txt"
INSTALL
      chmod 755 "${out}"
      ;;
    *)
      printf 'downloaded:%s\n' "${url}" > "${out}"
      ;;
  esac
  exit 0
fi

if [[ "${url}" == "http://localhost:8090/agent/status" ]]; then
  printf '{"status":"ok"}\n'
  exit 0
fi

exit 1
`)

	writeExecutable(t, filepath.Join(binDir, "systemctl"), `#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >> "${LABTETHER_TEST_LOG_DIR}/systemctl.log"
if [[ "${1:-}" == "is-active" ]]; then
  exit 1
fi
exit 0
`)

	writeExecutable(t, filepath.Join(binDir, "apt-get"), `#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >> "${LABTETHER_TEST_LOG_DIR}/apt-get.log"
exit 0
`)

	writeExecutable(t, filepath.Join(binDir, "uname"), `#!/bin/bash
set -euo pipefail
printf 'x86_64\n'
`)

	writeExecutable(t, filepath.Join(binDir, "openssl"), `#!/bin/bash
set -euo pipefail
infile=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -in)
      infile="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
cat "${infile}"
`)

	writeExecutable(t, filepath.Join(binDir, "sha256sum"), `#!/bin/bash
set -euo pipefail
cat >/dev/null
printf '%s  -\n' "${LABTETHER_TEST_CA_FINGERPRINT}"
`)

	writeExecutable(t, filepath.Join(binDir, "update-ca-certificates"), `#!/bin/bash
set -euo pipefail
printf 'update-ca-certificates\n' >> "${LABTETHER_TEST_LOG_DIR}/ca-tools.log"
`)

	writeExecutable(t, filepath.Join(binDir, "update-ca-trust"), `#!/bin/bash
set -euo pipefail
printf 'update-ca-trust %s\n' "$*" >> "${LABTETHER_TEST_LOG_DIR}/ca-tools.log"
`)

	env := append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"LABTETHER_TEST_LOG_DIR="+logDir,
		"LABTETHER_TEST_CA_FINGERPRINT="+expectedFingerprint,
	)
	return root, env
}

func rewriteAgentScriptForHarness(script, root string) string {
	replacer := strings.NewReplacer(
		`if [[ "${EUID}" -ne 0 ]]; then`, `if false; then`,
		`ACTUAL_CA_FINGERPRINT="${ACTUAL_CA_FINGERPRINT,,}"`, `ACTUAL_CA_FINGERPRINT="$(printf '%s' "${ACTUAL_CA_FINGERPRINT}" | tr '[:upper:]' '[:lower:]')"`,
		"/etc/systemd/system/labtether-agent.service", filepath.Join(root, "etc/systemd/system/labtether-agent.service"),
		"/usr/local/share/ca-certificates/labtether-ca.crt", filepath.Join(root, "usr/local/share/ca-certificates/labtether-ca.crt"),
		"/etc/pki/ca-trust/source/anchors/labtether-ca.crt", filepath.Join(root, "etc/pki/ca-trust/source/anchors/labtether-ca.crt"),
		"/usr/local/bin/labtether-agent", filepath.Join(root, "usr/local/bin/labtether-agent"),
		"/usr/local/bin/labtether", filepath.Join(root, "usr/local/bin/labtether"),
		"/etc/systemd/system", filepath.Join(root, "etc/systemd/system"),
		"/usr/local/share/ca-certificates", filepath.Join(root, "usr/local/share/ca-certificates"),
		"/etc/pki/ca-trust/source/anchors", filepath.Join(root, "etc/pki/ca-trust/source/anchors"),
		"/usr/local/bin", filepath.Join(root, "usr/local/bin"),
		"/etc/labtether", filepath.Join(root, "etc/labtether"),
	)
	return replacer.Replace(script)
}

func runGeneratedShellScript(t *testing.T, script string, env []string, args ...string) (string, error) {
	t.Helper()

	scriptPath := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write shell script: %v", err)
	}

	cmd := exec.Command("bash", append([]string{scriptPath}, args...)...)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func readFileIfExists(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "<missing>"
	}
	return string(data)
}

func dumpTree(t *testing.T, root string) string {
	t.Helper()

	var paths []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		paths = append(paths, rel)
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(paths)
	return strings.Join(paths, "\n")
}

func seedInstalledAgentState(t *testing.T, root string) {
	t.Helper()

	files := map[string]string{
		filepath.Join(root, "usr/local/bin/labtether-agent"):              "#!/bin/bash\nexit 0\n",
		filepath.Join(root, "etc/labtether/agent.env"):                    "LABTETHER_WS_URL=wss://hub.example.com/ws/agent\n",
		filepath.Join(root, "etc/labtether/agent-token"):                  "persisted-token\n",
		filepath.Join(root, "etc/labtether/device-key"):                   "device-key\n",
		filepath.Join(root, "etc/labtether/device-key.pub"):               "device-key-pub\n",
		filepath.Join(root, "etc/labtether/device-fingerprint"):           "sha256:fingerprint\n",
		filepath.Join(root, "etc/systemd/system/labtether-agent.service"): "[Unit]\nDescription=LabTether Agent\n",
	}
	for path, contents := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, "labtether-agent") {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(contents), mode); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}
