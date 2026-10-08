package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptSummaryRequiresPrivateNonemptyRegularAgentToken(t *testing.T) {
	tests := []struct {
		name         string
		prepareToken func(t *testing.T, tokenPath string)
	}{
		{name: "fresh install"},
		{
			name: "empty token file",
			prepareToken: func(t *testing.T, tokenPath string) {
				t.Helper()
				if err := os.WriteFile(tokenPath, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unreadable token file",
			prepareToken: func(t *testing.T, tokenPath string) {
				t.Helper()
				if err := os.WriteFile(tokenPath, []byte("must-not-appear"), 0o200); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "non-private token file",
			prepareToken: func(t *testing.T, tokenPath string) {
				t.Helper()
				if err := os.WriteFile(tokenPath, []byte("must-not-appear"), 0o640); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "non-regular token path",
			prepareToken: func(t *testing.T, tokenPath string) {
				t.Helper()
				if err := os.Mkdir(tokenPath, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink token path",
			prepareToken: func(t *testing.T, tokenPath string) {
				t.Helper()
				target := filepath.Join(filepath.Dir(tokenPath), "symlink-target")
				if err := os.WriteFile(target, []byte("must-not-appear"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, tokenPath); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, env := newAgentScriptHarness(t, strings.Repeat("7", 64))
			tokenPath := filepath.Join(root, "etc/labtether/agent-token")
			if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.prepareToken != nil {
				tc.prepareToken(t, tokenPath)
			}

			script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
			output, err := runGeneratedShellScript(t, script, env, "--skip-vnc-prereqs")
			if err != nil {
				t.Fatalf("run install script: %v\noutput:\n%s", err, output)
			}
			if !strings.Contains(output, "Awaiting approval in LabTether console") {
				t.Fatalf("expected awaiting-approval summary, got:\n%s", output)
			}
			if strings.Contains(output, "Existing agent approval preserved") || strings.Contains(output, "must-not-appear") {
				t.Fatalf("summary claimed or exposed an unusable token:\n%s", output)
			}
		})
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
	if strings.Contains(output, "enroll-123") {
		t.Fatalf("install output exposed enrollment token value:\n%s", output)
	}
}
