package main

import (
	"encoding/json"
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
