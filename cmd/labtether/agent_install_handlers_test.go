package main

import (
	"github.com/labtether/labtether/internal/certmgr"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestHandleAgentInstallScript exercises the install script endpoint.
// Not parallel: mutates package-level tailscale function vars.
func TestHandleAgentInstallScript(t *testing.T) {
	originalLookPath := tailscaleLookPath
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleFallbackPaths = originalFallbackPaths
	})
	tailscaleLookPath = func(string) (string, error) {
		return "", os.ErrNotExist
	}
	tailscaleFallbackPaths = func() []string { return nil }

	tests := []struct {
		name           string
		externalURL    string
		host           string
		tlsEnabled     bool
		remoteAddr     string
		forwardedHost  string
		forwardedProto string
		wantHubURL     string
		wantWSURL      string
		wantShebang    bool
		wantUninstall  bool
		wantPurge      bool
	}{
		{
			name:          "hub URL derived from Host header",
			externalURL:   "",
			host:          "192.168.1.10:8080",
			tlsEnabled:    false,
			wantHubURL:    "http://192.168.1.10:8080",
			wantWSURL:     "ws://192.168.1.10:8080/ws/agent",
			wantShebang:   true,
			wantUninstall: true,
			wantPurge:     true,
		},
		{
			name:          "external URL override",
			externalURL:   "https://labtether.example.com",
			host:          "ignored-host",
			tlsEnabled:    false,
			wantHubURL:    "https://labtether.example.com",
			wantWSURL:     "wss://labtether.example.com/ws/agent",
			wantShebang:   true,
			wantUninstall: true,
			wantPurge:     true,
		},
		{
			name:          "insecure external URL ignored when TLS enabled",
			externalURL:   "http://labtether.example.com:8080",
			host:          "secure.local:8443",
			tlsEnabled:    true,
			wantHubURL:    "https://secure.local:8443",
			wantWSURL:     "wss://secure.local:8443/ws/agent",
			wantShebang:   true,
			wantUninstall: true,
			wantPurge:     true,
		},
		{
			name:          "TLS enabled via tlsEnabled field",
			externalURL:   "",
			host:          "lab.local:8443",
			tlsEnabled:    true,
			wantHubURL:    "https://lab.local:8443",
			wantWSURL:     "wss://lab.local:8443/ws/agent",
			wantShebang:   true,
			wantUninstall: true,
			wantPurge:     true,
		},
		{
			name:           "trusted HTTP console proxy origin",
			externalURL:    "",
			host:           "labtether:8443",
			tlsEnabled:     true,
			remoteAddr:     "127.0.0.1:40000",
			forwardedHost:  "console.example:3000",
			forwardedProto: "http",
			wantHubURL:     "http://console.example:3000",
			wantWSURL:      "ws://console.example:3000/ws/agent",
			wantShebang:    true,
			wantUninstall:  true,
			wantPurge:      true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := &apiServer{
				externalURL: tc.externalURL,
				tlsState:    TLSState{Enabled: tc.tlsEnabled},
			}

			req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
			req.Host = tc.host
			if tc.remoteAddr != "" {
				req.RemoteAddr = tc.remoteAddr
			}
			if tc.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tc.forwardedHost)
			}
			if tc.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tc.forwardedProto)
			}
			rec := httptest.NewRecorder()

			srv.handleAgentInstallScript(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (body: %q)", rec.Code, rec.Body.String())
			}

			body := rec.Body.String()

			ct := rec.Header().Get("Content-Type")
			if !strings.Contains(ct, "text/x-shellscript") {
				t.Errorf("Content-Type: got %q, want text/x-shellscript", ct)
			}

			if tc.wantShebang && !strings.HasPrefix(body, "#!/bin/bash") {
				t.Errorf("script does not start with #!/bin/bash")
			}

			if !strings.Contains(body, tc.wantHubURL) {
				t.Errorf("script does not contain hub URL %q", tc.wantHubURL)
			}

			if !strings.Contains(body, tc.wantWSURL) {
				t.Errorf("script does not contain WS URL %q", tc.wantWSURL)
			}

			if tc.wantUninstall && !strings.Contains(body, "--uninstall") {
				t.Errorf("script does not contain --uninstall support")
			}
			if tc.wantPurge && !strings.Contains(body, "--purge") {
				t.Errorf("script does not contain --purge support")
			}
			if !strings.Contains(body, "--docker-enabled") {
				t.Errorf("script does not contain --docker-enabled support")
			}
			if !strings.Contains(body, "--docker-endpoint") {
				t.Errorf("script does not contain --docker-endpoint support")
			}
			if !strings.Contains(body, "--files-root-mode") {
				t.Errorf("script does not contain --files-root-mode support")
			}
			if !strings.Contains(body, "--auto-update") {
				t.Errorf("script does not contain --auto-update support")
			}
			if !strings.Contains(body, "--force-update") {
				t.Errorf("script does not contain --force-update support")
			}
			if !strings.Contains(body, "--enrollment-token") {
				t.Errorf("script does not contain --enrollment-token support")
			}
			if !strings.Contains(body, "--enrollment-token-file") {
				t.Errorf("script does not contain file-backed enrollment token support")
			}
			if !strings.Contains(body, "--auto-install-vnc") {
				t.Errorf("script does not contain --auto-install-vnc support")
			}
			if !strings.Contains(body, "gstreamer1.0-tools") {
				t.Errorf("script does not contain GStreamer desktop prerequisite packages")
			}
			if !strings.Contains(body, "xdotool") {
				t.Errorf("script does not contain xdotool desktop prerequisite support")
			}
			if !strings.Contains(body, "--tls-skip-verify") {
				t.Errorf("script does not contain --tls-skip-verify support")
			}
			if !strings.Contains(body, "--tls-ca-file") {
				t.Errorf("script does not contain --tls-ca-file support")
			}
			if !strings.Contains(body, "labtether agent uninstall") {
				t.Errorf("script does not contain labtether uninstall helper command")
			}
		})
	}
}

func TestHandleAgentInstallScriptBuiltInCAUsesTrustedForwardedHTTPSOrigin(t *testing.T) {
	withHealthyTailscaleHTTPSUnavailable(t)
	withMockHubInterfaces(t, nil, map[int][]net.Addr{})

	ca, err := certmgr.GenerateCA()
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}
	srv := &apiServer{
		tlsState: TLSState{
			Enabled:   true,
			Source:    tlsSourceBuiltIn,
			CACertPEM: certmgr.CertPEM(ca.Cert),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/install.sh", nil)
	req.Host = "labtether:8443"
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("X-Forwarded-Host", "proxy.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	srv.handleAgentInstallScript(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %q)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"https://proxy.example",
		"wss://proxy.example/ws/agent",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("forwarded HTTPS installer missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"https://labtether:8443",
		"ca_fingerprint_sha256=",
		"/api/v1/agent/bootstrap.sh",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("forwarded HTTPS installer unexpectedly contains %q", forbidden)
		}
	}
}

// TestHandleAgentInstallScriptMethodNotAllowed verifies that non-GET methods are rejected.
func TestHandleAgentInstallScriptMethodNotAllowed(t *testing.T) {
	t.Parallel()

	srv := &apiServer{}
	req := httptest.NewRequest(http.MethodPost, "/install.sh", nil)
	rec := httptest.NewRecorder()

	srv.handleAgentInstallScript(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d, want 405", rec.Code)
	}
}

// Not parallel: stubs package-level tailscale function vars.
func TestHandleAgentBootstrapScript(t *testing.T) {
	originalLookPath := tailscaleLookPath
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleFallbackPaths = originalFallbackPaths
	})
	tailscaleLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	tailscaleFallbackPaths = func() []string { return nil }

	expectedFingerprint := strings.Repeat("a", 64)
	srv := &apiServer{
		tlsState: TLSState{Enabled: true},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256="+expectedFingerprint, nil)
	req.Host = "secure.local:8443"
	rec := httptest.NewRecorder()

	srv.handleAgentBootstrapScript(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %q)", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/x-shellscript") {
		t.Errorf("Content-Type: got %q, want text/x-shellscript", ct)
	}
	if !strings.Contains(body, "#!/bin/bash") {
		t.Errorf("script does not start with #!/bin/bash")
	}
	if !strings.Contains(body, "https://secure.local:8443") {
		t.Errorf("script does not include resolved hub URL")
	}
	if !strings.Contains(body, expectedFingerprint) {
		t.Errorf("script does not include expected CA fingerprint")
	}
	if !strings.Contains(body, "/api/v1/ca.crt") {
		t.Errorf("script does not include CA download endpoint")
	}
	if !strings.Contains(body, "/install.sh") {
		t.Errorf("script does not include install script download endpoint")
	}
}

func TestHandleAgentBootstrapScriptRejectsInvalidFingerprint(t *testing.T) {
	t.Parallel()

	srv := &apiServer{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=invalid", nil)
	rec := httptest.NewRecorder()

	srv.handleAgentBootstrapScript(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestHandleAgentBootstrapScriptMethodNotAllowed(t *testing.T) {
	t.Parallel()

	srv := &apiServer{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/bootstrap.sh", nil)
	rec := httptest.NewRecorder()

	srv.handleAgentBootstrapScript(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d, want 405", rec.Code)
	}
}

// TestGenerateInstallScript checks structural properties of the generated script.
func TestGenerateInstallScript(t *testing.T) {
	t.Parallel()

	hubURL := "http://192.168.1.100:8080"
	wsURL := "ws://192.168.1.100:8080/ws/agent"
	script := generateInstallScript(hubURL, wsURL)

	checks := []struct {
		desc    string
		snippet string
	}{
		{"shebang", "#!/bin/bash"},
		{"strict mode", "set -euo pipefail"},
		{"uninstall flag", "--uninstall"},
		{"purge flag", "--purge"},
		{"docker enabled flag", "--docker-enabled"},
		{"docker endpoint flag", "--docker-endpoint"},
		{"docker interval flag", "--docker-discovery-interval"},
		{"docker wizard flag", "--docker-wizard"},
		{"files root mode flag", "--files-root-mode"},
		{"auto update flag", "--auto-update"},
		{"force update flag", "--force-update"},
		{"enrollment token flag", "--enrollment-token"},
		{"enrollment token file flag", "--enrollment-token-file"},
		{"tls skip verify flag", "--tls-skip-verify"},
		{"tls ca file flag", "--tls-ca-file"},
		{"root check", "EUID"},
		{"systemd check", "systemctl"},
		{"curl/wget check", "DOWNLOADER"},
		{"arch detection", "uname -m"},
		{"amd64 mapping", "amd64"},
		{"arm64 mapping", "arm64"},
		{"binary download URL", "/api/v1/agent/binary?arch="},
		{"hub URL embedded", hubURL},
		{"ws URL embedded", wsURL},
		{"env file", "/etc/labtether/agent.env"},
		{"fingerprint file", "device-fingerprint"},
		{"device key file", "device-key"},
		{"device public key file", "device-key.pub"},
		{"LABTETHER_WS_URL", "LABTETHER_WS_URL"},
		{"LABTETHER_ENROLLMENT_TOKEN_FILE", "LABTETHER_ENROLLMENT_TOKEN_FILE"},
		{"LABTETHER_DOCKER_ENABLED", "LABTETHER_DOCKER_ENABLED"},
		{"LABTETHER_DOCKER_SOCKET", "LABTETHER_DOCKER_SOCKET"},
		{"LABTETHER_DOCKER_DISCOVERY_INTERVAL", "LABTETHER_DOCKER_DISCOVERY_INTERVAL"},
		{"LABTETHER_FILES_ROOT_MODE", "LABTETHER_FILES_ROOT_MODE"},
		{"LABTETHER_AUTO_UPDATE", "LABTETHER_AUTO_UPDATE"},
		{"labtether cli helper destination", "/usr/local/bin/labtether"},
		{"labtether cli helper marker", "LABTETHER_AGENT_WRAPPER=1"},
		{"labtether cli helper uninstall command", "labtether agent uninstall"},
		{"labtether cli helper purge command", "labtether agent purge"},
		{"LABTETHER_TLS_SKIP_VERIFY", "LABTETHER_TLS_SKIP_VERIFY"},
		{"LABTETHER_TLS_CA_FILE", "LABTETHER_TLS_CA_FILE"},
		{"forced update command", "update self --force"},
		{"preserve agent token", "agent-token"},
		{"docker connectivity check", "connected"},
		{"docker ui tip", "configure Add Device"},
		{"purge cleanup message", "uninstalled and purged"},
		{"systemd unit description", "LabTether Agent"},
		{"systemd protect home read-only", "ProtectHome=read-only"},
		{"systemd protect system off", "ProtectSystem=off"},
		{"systemd enable", "systemctl enable --now"},
		{"daemon-reload", "daemon-reload"},
		{"status check", "/agent/status"},
		{"fingerprint verify message", "Verify this fingerprint in LabTether before approving"},
		{"success message", "Auto-enrollment configured"},
	}

	for _, c := range checks {
		if !strings.Contains(script, c.snippet) {
			t.Errorf("missing %s: snippet %q not found in script", c.desc, c.snippet)
		}
	}
}

// TestHTTPURLToWS verifies http/https → ws/wss conversion.
func TestHTTPURLToWS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"http://localhost:8080", "ws://localhost:8080"},
		{"https://lab.example.com", "wss://lab.example.com"},
		{"https://lab.example.com:8443", "wss://lab.example.com:8443"},
		{"http://192.168.1.1:8080", "ws://192.168.1.1:8080"},
		{"no-scheme-host", "ws://no-scheme-host"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := httpURLToWS(tc.input)
			if got != tc.want {
				t.Errorf("httpURLToWS(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
