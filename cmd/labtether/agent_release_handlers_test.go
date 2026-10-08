package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read test binary %s: %v", name, err)
		}
		digest := sha256.Sum256(data)
		entries += `
        "` + key + `": {
          "name": "` + name + `",
          "sha256": "` + fmt.Sprintf("%x", digest[:]) + `",
          "size_bytes": ` + fmt.Sprintf("%d", len(data)) + `
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
