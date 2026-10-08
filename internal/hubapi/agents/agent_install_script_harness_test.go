package agents

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

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
if [[ "${1:-}" == "update" && "${2:-}" == "self" ]]; then
  printf 'test update output\n'
  exit "${LABTETHER_TEST_FORCE_UPDATE_EXIT:-0}"
fi
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

if [[ "${url}" == *"/api/v1/agent/releases/latest?arch="* ]]; then
  printf '{"sha256":"%s"}\n' "${LABTETHER_TEST_BINARY_SHA256}"
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
if [[ $# -gt 0 ]]; then
  printf '%s  %s\n' "${LABTETHER_TEST_BINARY_SHA256}" "$1"
  exit 0
fi
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
		"LABTETHER_TEST_BINARY_SHA256="+strings.Repeat("c", 64),
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
