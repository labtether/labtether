package agents

import (
	"os"
	"path/filepath"
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
	enrollmentTokenSource := filepath.Join(root, "fixtures", "enrollment-token")
	if err := os.WriteFile(enrollmentTokenSource, []byte("enroll-123\n"), 0o600); err != nil {
		t.Fatalf("write enrollment token file: %v", err)
	}

	script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
	env = append(env, "LABTETHER_LOW_POWER_MODE=true", "LABTETHER_LOG_STREAM_ENABLED=false")
	_, err := runGeneratedShellScript(t, script, env,
		"--docker-enabled", "true",
		"--docker-endpoint", "/var/run/docker.sock",
		"--docker-discovery-interval", "45",
		"--files-root-mode", "full",
		"--auto-update", "false",
		"--skip-vnc-prereqs",
		"--enrollment-token-file", enrollmentTokenSource,
		"--tls-ca-file", caPath,
	)
	if err != nil {
		t.Fatalf("run install script: %v", err)
	}

	envFile := mustReadFile(t, filepath.Join(root, "etc/labtether/agent.env"))
	if info, err := os.Stat(filepath.Join(root, "etc/labtether/agent.env")); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("agent environment file mode = %o, want 600", info.Mode().Perm())
	}
	for _, want := range []string{
		"LABTETHER_WS_URL=wss://hub.example.com/ws/agent",
		"LABTETHER_ENROLLMENT_TOKEN_FILE=" + filepath.Join(root, "etc/labtether/enrollment-token"),
		"LABTETHER_DOCKER_ENABLED=true",
		`LABTETHER_DOCKER_SOCKET="/var/run/docker.sock"`,
		"LABTETHER_DOCKER_DISCOVERY_INTERVAL=45",
		"LABTETHER_FILES_ROOT_MODE=full",
		"LABTETHER_AUTO_UPDATE=false",
		"LABTETHER_LOW_POWER_MODE=true",
		"LABTETHER_LOG_STREAM_ENABLED=false",
		"LABTETHER_TLS_CA_FILE=" + caPath,
	} {
		if !strings.Contains(envFile, want) {
			t.Fatalf("expected env file to contain %q, got:\n%s", want, envFile)
		}
	}
	if strings.Contains(envFile, "LABTETHER_ENROLLMENT_TOKEN=") {
		t.Fatal("environment file contains a plaintext enrollment token")
	}
	managedEnrollmentToken := filepath.Join(root, "etc/labtether/enrollment-token")
	if got := mustReadFile(t, managedEnrollmentToken); got != "enroll-123\n" {
		t.Fatalf("managed enrollment token = %q", got)
	}
	if info, err := os.Stat(managedEnrollmentToken); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("managed enrollment token mode = %o, want 600", info.Mode().Perm())
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

func TestInstallScriptForceUpdateUsesPrivateLog(t *testing.T) {
	for _, tt := range []struct {
		name     string
		exitCode string
	}{
		{name: "success", exitCode: "0"},
		{name: "failure", exitCode: "1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, env := newAgentScriptHarness(t, strings.Repeat("a", 64))
			env = append(env, "LABTETHER_TEST_FORCE_UPDATE_EXIT="+tt.exitCode)
			script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
			if strings.Contains(script, "/tmp/labtether-force-update.log") {
				t.Fatal("installer contains the predictable shared log path")
			}
			output, err := runGeneratedShellScript(t, script, env, "--force-update", "--skip-vnc-prereqs")
			if err != nil {
				t.Fatalf("force-update install failed: %v\n%s", err, output)
			}
			if !strings.Contains(mustReadFile(t, filepath.Join(root, "logs/agent-binary.log")), "update self --force") {
				t.Fatal("force update was not run")
			}
			if tt.exitCode != "0" && !strings.Contains(output, "test update output") {
				t.Fatalf("failed update output was hidden: %s", output)
			}
			matches, err := filepath.Glob(filepath.Join(root, "etc/labtether/.force-update.*"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("force-update log was not cleaned up: %v, %v", matches, err)
			}
		})
	}
}

func TestBootstrapScriptExecutesPinnedInstallFlow(t *testing.T) {
	expectedFingerprint := strings.Repeat("b", 64)
	root, env := newAgentScriptHarness(t, expectedFingerprint)
	enrollmentTokenSource := filepath.Join(root, "bootstrap-enrollment-token")
	if err := os.WriteFile(enrollmentTokenSource, []byte("bootstrap-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	script := rewriteAgentScriptForHarness(GenerateBootstrapScript("https://hub.example.com", expectedFingerprint), root)
	output, err := runGeneratedShellScript(t, script, env, "--enrollment-token-file", enrollmentTokenSource)
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

	for _, want := range []string{"--tls-ca-file", caPath, "--enrollment-token-file", enrollmentTokenSource} {
		if !strings.Contains(installArgs, want) {
			t.Fatalf("expected bootstrap-installed script args to contain %q, got:\n%s", want, installArgs)
		}
	}
}

func TestInstallScriptRejectsWorldReadableEnrollmentTokenFile(t *testing.T) {
	root, env := newAgentScriptHarness(t, strings.Repeat("f", 64))
	tokenFile := filepath.Join(root, "enrollment-token")
	if err := os.WriteFile(tokenFile, []byte("enroll-123\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
	output, err := runGeneratedShellScript(t, script, env, "--enrollment-token-file", tokenFile)
	if err == nil {
		t.Fatal("expected insecure enrollment token file mode to be rejected")
	}
	if !strings.Contains(output, "chmod 600") {
		t.Fatalf("expected private-mode guidance, got:\n%s", output)
	}
}

func TestInstallScriptEnrollmentTokenFileLineEndings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "no terminal newline", content: "enroll-123"},
		{name: "terminal LF", content: "enroll-123\n"},
		{name: "terminal CRLF", content: "enroll-123\r\n"},
	} {
		t.Run("accepts "+tc.name, func(t *testing.T) {
			root, env := newAgentScriptHarness(t, strings.Repeat("f", 64))
			tokenFile := filepath.Join(root, "enrollment-token")
			if err := os.WriteFile(tokenFile, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}

			script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
			if output, err := runGeneratedShellScript(t, script, env, "--skip-vnc-prereqs", "--enrollment-token-file", tokenFile); err != nil {
				t.Fatalf("expected token file to be accepted: %v\n%s", err, output)
			}
			if got := mustReadFile(t, filepath.Join(root, "etc/labtether/enrollment-token")); got != "enroll-123\n" {
				t.Fatalf("managed enrollment token = %q, want one normalized terminal LF", got)
			}
		})
	}

	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "embedded LF", content: "enroll\n123"},
		{name: "embedded CRLF", content: "enroll\r\n123"},
		{name: "two terminal LF sequences", content: "enroll-123\n\n"},
		{name: "two terminal CRLF sequences", content: "enroll-123\r\n\r\n"},
		{name: "terminal CR", content: "enroll-123\r"},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			root, env := newAgentScriptHarness(t, strings.Repeat("f", 64))
			tokenFile := filepath.Join(root, "enrollment-token")
			if err := os.WriteFile(tokenFile, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}

			script := rewriteAgentScriptForHarness(GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent"), root)
			output, err := runGeneratedShellScript(t, script, env, "--skip-vnc-prereqs", "--enrollment-token-file", tokenFile)
			if err == nil {
				t.Fatal("expected token file with disallowed line endings to be rejected")
			}
			if !strings.Contains(output, "exactly one token with at most one terminal newline") {
				t.Fatalf("expected line-ending rejection, got:\n%s", output)
			}
		})
	}
}

func TestGeneratedScriptsShellQuoteInterpolatedLiterals(t *testing.T) {
	hubURL := `https://hub.example.com/path/$(touch injected)'suffix`
	wsURL := `wss://hub.example.com/path/$(touch injected)'suffix/ws/agent`

	installScript := GenerateInstallScript(hubURL, wsURL)
	for _, want := range []string{
		`HUB_URL='https://hub.example.com/path/$(touch injected)'"'"'suffix'`,
		`WS_URL='wss://hub.example.com/path/$(touch injected)'"'"'suffix/ws/agent'`,
	} {
		if !strings.Contains(installScript, want) {
			t.Fatalf("expected install script to contain shell-quoted literal %q, got:\n%s", want, installScript)
		}
	}
	for _, forbidden := range []string{
		`HUB_URL="https://hub.example.com/path/$(touch injected)'suffix"`,
		`WS_URL="wss://hub.example.com/path/$(touch injected)'suffix/ws/agent"`,
	} {
		if strings.Contains(installScript, forbidden) {
			t.Fatalf("install script contains unsafe double-quoted literal %q", forbidden)
		}
	}

	bootstrapScript := GenerateBootstrapScript(hubURL, strings.Repeat("a", 64))
	if !strings.Contains(bootstrapScript, `HUB_URL='https://hub.example.com/path/$(touch injected)'"'"'suffix'`) {
		t.Fatalf("expected bootstrap script to shell-quote HUB_URL, got:\n%s", bootstrapScript)
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
	output, err := runGeneratedShellScript(t, script, env, "--skip-vnc-prereqs", "--tls-ca-file", caPath)
	if err != nil {
		t.Fatalf("run reinstall script: %v", err)
	}

	if got := mustReadFile(t, tokenPath); !strings.Contains(got, "persisted-token") {
		t.Fatalf("expected persisted token to survive reinstall, got %q", got)
	}
	if !strings.Contains(output, "Existing agent approval preserved") || !strings.Contains(output, "reconnecting to LabTether") {
		t.Fatalf("expected preserved-enrollment summary, got:\n%s", output)
	}
	if strings.Contains(output, "persisted-token") {
		t.Fatalf("install output exposed persisted token value:\n%s", output)
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
