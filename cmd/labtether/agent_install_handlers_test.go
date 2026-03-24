package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
)

// writeTestManifestForBinary writes an agent-manifest.json into dir that maps
// the given os/arch to binaryName.
func writeTestManifestForBinary(t *testing.T, dir, agentOS, arch, binaryName string) {
	t.Helper()
	key := agentOS + "-" + arch
	manifest := `{
  "schema_version": 1,
  "generated_at": "2026-01-01T00:00:00Z",
  "hub_version": "test",
  "agents": {
    "labtether-agent": {
      "version": "0.0.0-test",
      "repo": "labtether/labtether-agent",
      "binaries": {
        "` + key + `": {
          "name": "` + binaryName + `",
          "sha256": "0000000000000000000000000000000000000000000000000000000000000000",
          "size_bytes": 0
        }
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "agent-manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatalf("write test manifest: %v", err)
	}
}

// writeTestManifestForBinaries writes an agent-manifest.json into dir that maps
// multiple os/arch pairs to their respective binary names.
func writeTestManifestForBinaries(t *testing.T, dir string, binaries map[string]string) {
	t.Helper()
	entries := ""
	i := 0
	for key, name := range binaries {
		if i > 0 {
			entries += ","
		}
		entries += `
        "` + key + `": {
          "name": "` + name + `",
          "sha256": "0000000000000000000000000000000000000000000000000000000000000000",
          "size_bytes": 0
        }`
		i++
	}
	manifest := `{
  "schema_version": 1,
  "generated_at": "2026-01-01T00:00:00Z",
  "hub_version": "test",
  "agents": {
    "labtether-agent": {
      "version": "0.0.0-test",
      "repo": "labtether/labtether-agent",
      "binaries": {` + entries + `
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "agent-manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatalf("write test manifest: %v", err)
	}
}

// TestHandleAgentBinary exercises the /api/v1/agent/binary endpoint.
func TestHandleAgentBinary(t *testing.T) {
	t.Parallel()

	// Create a temp directory with fake binaries and a manifest for test cases that expect a hit.
	dir := t.TempDir()
	binaries := make(map[string]string)
	for _, arch := range []string{"amd64", "arm64"} {
		name := "labtether-agent-linux-" + arch
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake-binary-"+arch), 0755); err != nil {
			t.Fatalf("setup: write %s: %v", name, err)
		}
		binaries["linux-"+arch] = name
	}
	writeTestManifestForBinaries(t, dir, binaries)
	dirCache := &agentspkg.AgentCache{RuntimeDir: dir, BakedInDir: dir}
	if err := dirCache.LoadManifest(); err != nil {
		t.Fatalf("setup: load manifest: %v", err)
	}

	// emptyDir has no binaries and no manifest — used for the "not found" sub-test.
	emptyDir := t.TempDir()
	writeTestManifestForBinary(t, emptyDir, "linux", "amd64", "labtether-agent-linux-amd64")
	emptyCache := &agentspkg.AgentCache{RuntimeDir: emptyDir, BakedInDir: emptyDir}
	if err := emptyCache.LoadManifest(); err != nil {
		t.Fatalf("setup: load empty manifest: %v", err)
	}

	tests := []struct {
		name       string
		cache      *agentspkg.AgentCache
		arch       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid amd64",
			cache:      dirCache,
			arch:       "amd64",
			wantStatus: http.StatusOK,
			wantBody:   "fake-binary-amd64",
		},
		{
			name:       "valid arm64",
			cache:      dirCache,
			arch:       "arm64",
			wantStatus: http.StatusOK,
			wantBody:   "fake-binary-arm64",
		},
		{
			name:       "missing arch param",
			cache:      dirCache,
			arch:       "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid arch",
			cache:      dirCache,
			arch:       "mips",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "arch not available",
			cache:      emptyCache,
			arch:       "amd64",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := &apiServer{agentCache: tc.cache}

			path := "/api/v1/agent/binary"
			if tc.arch != "" {
				path += "?arch=" + tc.arch
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			srv.handleAgentBinary(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d (body: %q)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body: want %q in response, got %q", tc.wantBody, rec.Body.String())
			}
		})
	}
}

// TestHandleAgentBinaryMethodNotAllowed verifies that non-GET methods are rejected.
func TestHandleAgentBinaryMethodNotAllowed(t *testing.T) {
	t.Parallel()

	srv := &apiServer{agentCache: &agentspkg.AgentCache{RuntimeDir: t.TempDir(), BakedInDir: t.TempDir()}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/binary?arch=amd64", nil)
	rec := httptest.NewRecorder()

	srv.handleAgentBinary(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d, want 405", rec.Code)
	}
}

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
		name          string
		externalURL   string
		host          string
		tlsEnabled    bool
		wantHubURL    string
		wantWSURL     string
		wantShebang   bool
		wantUninstall bool
		wantPurge     bool
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
		{"LABTETHER_ENROLLMENT_TOKEN", "LABTETHER_ENROLLMENT_TOKEN"},
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


func TestHandleAgentReleaseLatest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "labtether-agent-linux-amd64"), []byte("agent-binary"), 0755); err != nil {
		t.Fatalf("setup: write binary: %v", err)
	}
	writeTestManifestForBinary(t, dir, "linux", "amd64", "labtether-agent-linux-amd64")
	cache := &agentspkg.AgentCache{RuntimeDir: dir, BakedInDir: dir}
	if err := cache.LoadManifest(); err != nil {
		t.Fatalf("setup: load manifest: %v", err)
	}

	srv := &apiServer{
		agentCache:  cache,
		externalURL: "https://labtether.example.com",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/releases/latest?os=linux&arch=amd64", nil)
	rec := httptest.NewRecorder()

	srv.handleAgentReleaseLatest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}

	var payload struct {
		Version string `json:"version"`
		OS      string `json:"os"`
		Arch    string `json:"arch"`
		SHA256  string `json:"sha256"`
		URL     string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.OS != "linux" || payload.Arch != "amd64" {
		t.Fatalf("unexpected os/arch: %+v", payload)
	}
	if payload.SHA256 == "" || len(payload.SHA256) != 64 {
		t.Fatalf("expected sha256 hex digest, got %q", payload.SHA256)
	}
	if payload.Version == "" {
		t.Fatalf("expected non-empty version")
	}
	if !strings.Contains(payload.URL, "/api/v1/agent/binary?os=linux&arch=amd64") {
		t.Fatalf("unexpected download URL %q", payload.URL)
	}
}

func TestHandleAgentReleaseLatestRejectsNonGET(t *testing.T) {
	t.Parallel()

	srv := &apiServer{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/releases/latest?os=linux&arch=amd64", nil)
	rec := httptest.NewRecorder()

	srv.handleAgentReleaseLatest(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleAgentReleaseLatestRejectsMissingArch(t *testing.T) {
	t.Parallel()

	srv := &apiServer{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/releases/latest?os=linux", nil)
	rec := httptest.NewRecorder()

	srv.handleAgentReleaseLatest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "arch query parameter is required") {
		t.Fatalf("unexpected response body %q", rec.Body.String())
	}
}

func TestHandleAgentReleaseLatestRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestManifestForBinary(t, dir, "linux", "amd64", "labtether-agent-linux-amd64")
	cache := &agentspkg.AgentCache{RuntimeDir: dir, BakedInDir: dir}
	if err := cache.LoadManifest(); err != nil {
		t.Fatalf("setup: load manifest: %v", err)
	}

	srv := &apiServer{agentCache: cache}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/releases/latest?os=linux&arch=mips64", nil)
	rec := httptest.NewRecorder()

	srv.handleAgentReleaseLatest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAgentReleaseLatestReturnsNotFoundWhenBinaryMissing(t *testing.T) {
	t.Parallel()

	// Manifest exists with an entry, but the actual binary file is missing.
	dir := t.TempDir()
	writeTestManifestForBinary(t, dir, "linux", "amd64", "labtether-agent-linux-amd64")
	cache := &agentspkg.AgentCache{RuntimeDir: dir, BakedInDir: dir}
	if err := cache.LoadManifest(); err != nil {
		t.Fatalf("setup: load manifest: %v", err)
	}

	srv := &apiServer{agentCache: cache}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/releases/latest?os=linux&arch=amd64", nil)
	rec := httptest.NewRecorder()

	srv.handleAgentReleaseLatest(rec, req)

	// With manifest-driven lookup, the release metadata is served from the manifest
	// even when the binary file is absent. The release endpoint returns 200 with
	// manifest data (version, sha256, URL). Only the binary download endpoint
	// would return 404.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}

func TestHandleAgentReleaseLatestNoManifest(t *testing.T) {
	t.Parallel()

	// No manifest loaded — should return 503.
	cache := &agentspkg.AgentCache{RuntimeDir: t.TempDir(), BakedInDir: t.TempDir()}
	srv := &apiServer{agentCache: cache}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/releases/latest?os=linux&arch=amd64", nil)
	rec := httptest.NewRecorder()

	srv.handleAgentReleaseLatest(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}

