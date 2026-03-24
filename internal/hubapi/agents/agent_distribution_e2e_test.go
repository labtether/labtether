package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/servicehttp"
)

// TestAgentDistributionE2E exercises the complete agent distribution flow
// end-to-end: manifest loading, binary serving, release metadata, manifest
// endpoint, and cache refresh.
func TestAgentDistributionE2E(t *testing.T) {
	// --- Setup: create a real binary, compute its SHA256, write a manifest ---
	dir := t.TempDir()

	binaryContent := []byte("#!/bin/sh\necho 'labtether-agent v2026.1'\n")
	binaryName := "labtether-agent-linux-amd64"
	if err := os.WriteFile(filepath.Join(dir, binaryName), binaryContent, 0755); err != nil {
		t.Fatal(err)
	}

	hash := sha256.Sum256(binaryContent)
	sha256Hex := hex.EncodeToString(hash[:])

	manifest := AgentManifest{
		SchemaVersion: 1,
		GeneratedAt:   "2026-03-24T12:00:00Z",
		HubVersion:    "v2026.1",
		Agents: map[string]AgentEntry{
			"labtether-agent": {
				Version: "v2026.1",
				Repo:    "labtether/labtether-agent",
				Binaries: map[string]BinaryEntry{
					"linux-amd64": {
						Name:      binaryName,
						SHA256:    sha256Hex,
						SizeBytes: int64(len(binaryContent)),
						URL:       fmt.Sprintf("https://github.com/labtether/labtether-agent/releases/download/v2026.1/%s", binaryName),
					},
				},
			},
			"labtether-mac": {
				Version: "v2026.1",
				Repo:    "labtether/labtether-mac",
				Type:    "metadata-only",
				Binaries: map[string]BinaryEntry{
					"darwin-universal": {
						Name:   "labtether-agent-macos-universal.tar.gz",
						SHA256: "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
						URL:    "https://github.com/labtether/labtether-mac/releases/download/v2026.1/labtether-agent-macos-universal.tar.gz",
					},
				},
			},
		},
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFilename), manifestData, 0644); err != nil {
		t.Fatal(err)
	}

	// --- Setup: create AgentCache and load manifest ---
	cache := &AgentCache{
		RuntimeDir: dir,
		BakedInDir: dir,
	}
	if err := cache.LoadManifest(); err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	m := cache.Manifest()
	if m == nil {
		t.Fatal("manifest is nil after LoadManifest")
	}
	if m.GoAgentVersion() != "v2026.1" {
		t.Fatalf("GoAgentVersion = %q, want v2026.1", m.GoAgentVersion())
	}

	// --- Setup: create Deps with real handlers ---
	deps := &Deps{
		AgentCache: cache,
		ResolveHubURL: func(r *http.Request) string {
			return "https://hub.example.com:8443"
		},
	}

	// --- Test 1: GET /api/v1/agent/binary?arch=amd64 ---
	t.Run("binary_download", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/binary?arch=amd64", nil)
		w := httptest.NewRecorder()
		deps.HandleAgentBinary(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
		}

		ct := w.Header().Get("Content-Type")
		if ct != "application/octet-stream" {
			t.Errorf("Content-Type = %q, want application/octet-stream", ct)
		}

		cd := w.Header().Get("Content-Disposition")
		if !strings.Contains(cd, binaryName) {
			t.Errorf("Content-Disposition = %q, want to contain %q", cd, binaryName)
		}

		body := w.Body.Bytes()
		if string(body) != string(binaryContent) {
			t.Errorf("body length = %d, want %d", len(body), len(binaryContent))
		}

		// Verify the SHA256 of what was served matches the manifest
		servedHash := sha256.Sum256(body)
		servedSHA := hex.EncodeToString(servedHash[:])
		if servedSHA != sha256Hex {
			t.Errorf("served SHA256 = %s, manifest SHA256 = %s", servedSHA, sha256Hex)
		}
	})

	// --- Test 2: GET /api/v1/agent/binary?arch=amd64&os=linux (explicit os) ---
	t.Run("binary_download_explicit_os", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/binary?os=linux&arch=amd64", nil)
		w := httptest.NewRecorder()
		deps.HandleAgentBinary(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	// --- Test 3: GET /api/v1/agent/binary without arch → 400 ---
	t.Run("binary_missing_arch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/binary", nil)
		w := httptest.NewRecorder()
		deps.HandleAgentBinary(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	// --- Test 4: GET /api/v1/agent/binary?arch=arm64 → 400 (not in manifest) ---
	t.Run("binary_unknown_arch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/binary?arch=arm64", nil)
		w := httptest.NewRecorder()
		deps.HandleAgentBinary(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
		}
	})

	// --- Test 5: GET /api/v1/agent/binary?os=windows&arch=amd64 → 400 (not in manifest) ---
	t.Run("binary_unknown_os", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/binary?os=windows&arch=amd64", nil)
		w := httptest.NewRecorder()
		deps.HandleAgentBinary(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	// --- Test 6: GET /api/v1/agent/releases/latest?arch=amd64 ---
	t.Run("release_latest", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/releases/latest?arch=amd64", nil)
		w := httptest.NewRecorder()
		deps.HandleAgentReleaseLatest(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
		}

		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		// Check all expected fields
		checks := map[string]string{
			"version":      "v2026.1",
			"os":           "linux",
			"arch":         "amd64",
			"binary_name":  binaryName,
			"sha256":       sha256Hex,
			"published_at": "2026-03-24T12:00:00Z",
		}
		for key, want := range checks {
			got, ok := result[key].(string)
			if !ok {
				t.Errorf("missing or non-string field %q", key)
				continue
			}
			if got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}

		// Check size_bytes
		sizeBytes, ok := result["size_bytes"].(float64)
		if !ok {
			t.Error("missing size_bytes")
		} else if int64(sizeBytes) != int64(len(binaryContent)) {
			t.Errorf("size_bytes = %v, want %d", sizeBytes, len(binaryContent))
		}

		// Check URL points at hub
		urlStr, ok := result["url"].(string)
		if !ok {
			t.Error("missing url")
		} else if !strings.HasPrefix(urlStr, "https://hub.example.com:8443/api/v1/agent/binary") {
			t.Errorf("url = %q, want hub URL prefix", urlStr)
		}
	})

	// --- Test 7: GET /api/v1/agent/manifest ---
	t.Run("manifest_endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/manifest", nil)
		w := httptest.NewRecorder()
		deps.HandleAgentManifest(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var result AgentManifest
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		if result.SchemaVersion != 1 {
			t.Errorf("schema_version = %d, want 1", result.SchemaVersion)
		}
		if result.HubVersion != "v2026.1" {
			t.Errorf("hub_version = %q", result.HubVersion)
		}

		// Verify Go agent entry
		goAgent, ok := result.Agents["labtether-agent"]
		if !ok {
			t.Fatal("missing labtether-agent")
		}
		if goAgent.Version != "v2026.1" {
			t.Errorf("go agent version = %q", goAgent.Version)
		}
		if goAgent.Type != "" {
			t.Errorf("go agent type = %q, want empty", goAgent.Type)
		}

		// Verify macOS metadata-only entry
		macAgent, ok := result.Agents["labtether-mac"]
		if !ok {
			t.Fatal("missing labtether-mac")
		}
		if macAgent.Type != "metadata-only" {
			t.Errorf("mac agent type = %q, want metadata-only", macAgent.Type)
		}
	})

	// --- Test 8: POST /api/v1/agent/cache/refresh ---
	t.Run("cache_refresh", func(t *testing.T) {
		// Update the manifest on disk to v2026.2
		manifest.HubVersion = "v2026.2"
		manifest.Agents["labtether-agent"] = AgentEntry{
			Version: "v2026.2",
			Repo:    "labtether/labtether-agent",
			Binaries: map[string]BinaryEntry{
				"linux-amd64": manifest.Agents["labtether-agent"].Binaries["linux-amd64"],
			},
		}
		updatedData, _ := json.MarshalIndent(manifest, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, ManifestFilename), updatedData, 0644); err != nil {
			t.Fatal(err)
		}

		// Verify cache still serves old version
		if v := deps.AgentCache.Manifest().GoAgentVersion(); v != "v2026.1" {
			t.Fatalf("pre-refresh version = %q, want v2026.1", v)
		}

		// Refresh
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/cache/refresh", nil)
		w := httptest.NewRecorder()
		deps.HandleAgentCacheRefresh(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
		}

		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["status"] != "refreshed" {
			t.Errorf("status = %q", result["status"])
		}

		// Verify cache now serves new version
		if v := deps.AgentCache.Manifest().GoAgentVersion(); v != "v2026.2" {
			t.Errorf("post-refresh version = %q, want v2026.2", v)
		}
	})

	// --- Test 9: Verify no manifest → 503 ---
	t.Run("no_manifest_503", func(t *testing.T) {
		emptyCache := &AgentCache{RuntimeDir: t.TempDir(), BakedInDir: t.TempDir()}
		emptyDeps := &Deps{
			AgentCache:    emptyCache,
			ResolveHubURL: func(r *http.Request) string { return "https://hub.example.com" },
		}

		// Binary endpoint → 503
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/binary?arch=amd64", nil)
		w := httptest.NewRecorder()
		emptyDeps.HandleAgentBinary(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("binary status = %d, want 503", w.Code)
		}

		// Release latest → 503
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agent/releases/latest?arch=amd64", nil)
		w = httptest.NewRecorder()
		emptyDeps.HandleAgentReleaseLatest(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("release status = %d, want 503", w.Code)
		}

		// Manifest endpoint → 503
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agent/manifest", nil)
		w = httptest.NewRecorder()
		emptyDeps.HandleAgentManifest(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("manifest status = %d, want 503", w.Code)
		}
	})

	// --- Test 10: Install script includes SHA256 verification ---
	t.Run("install_script_has_sha256", func(t *testing.T) {
		script := GenerateInstallScript("https://hub.example.com", "wss://hub.example.com/ws/agent")
		if !strings.Contains(script, "sha256") && !strings.Contains(script, "SHA256") {
			t.Error("install script does not contain SHA256 verification")
		}
		if !strings.Contains(script, "releases/latest") {
			t.Error("install script does not reference releases/latest endpoint")
		}
	})

	// --- Test 11: Version status comparison ---
	t.Run("version_status", func(t *testing.T) {
		if s := DetermineAgentVersionStatus("v2026.1", "v2026.1"); s != "up_to_date" {
			t.Errorf("same version = %q, want up_to_date", s)
		}
		if s := DetermineAgentVersionStatus("v2026.0", "v2026.1"); s != "update_available" {
			t.Errorf("old version = %q, want update_available", s)
		}
		if s := DetermineAgentVersionStatus("", "v2026.1"); s != "unknown" {
			t.Errorf("empty version = %q, want unknown", s)
		}
	})

	// --- Test 12: LatestAgentVersionForPlatform reads from manifest ---
	t.Run("latest_version_from_manifest", func(t *testing.T) {
		version, publishedAt, err := deps.LatestAgentVersionForPlatform("linux", "amd64")
		if err != nil {
			t.Fatal(err)
		}
		// After cache refresh in test 8, version is v2026.2
		if version != "v2026.2" {
			t.Errorf("version = %q", version)
		}
		if publishedAt == "" {
			t.Error("published_at is empty")
		}
	})

	// --- Test 13: Two-tier cache resolution (runtime > baked-in) ---
	t.Run("cache_two_tier", func(t *testing.T) {
		runtimeDir := t.TempDir()
		bakedInDir := t.TempDir()

		// Only put binary in baked-in dir
		if err := os.WriteFile(filepath.Join(bakedInDir, "test-binary"), []byte("baked"), 0755); err != nil {
			t.Fatal(err)
		}

		c := &AgentCache{RuntimeDir: runtimeDir, BakedInDir: bakedInDir}

		// Should find in baked-in
		path, err := c.ResolveBinaryPath("test-binary")
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "baked" {
			t.Errorf("got %q, want baked", string(data))
		}

		// Now put a newer one in runtime
		if err := os.WriteFile(filepath.Join(runtimeDir, "test-binary"), []byte("runtime"), 0755); err != nil {
			t.Fatal(err)
		}

		// Should prefer runtime
		path, err = c.ResolveBinaryPath("test-binary")
		if err != nil {
			t.Fatal(err)
		}
		data, _ = os.ReadFile(path)
		if string(data) != "runtime" {
			t.Errorf("got %q, want runtime", string(data))
		}
	})

	// --- Test 14: Simulate full download + SHA256 verify flow ---
	t.Run("download_and_verify", func(t *testing.T) {
		// Step 1: Get release metadata
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/releases/latest?arch=amd64", nil)
		w := httptest.NewRecorder()
		deps.HandleAgentReleaseLatest(w, req)

		var meta map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &meta); err != nil {
			t.Fatal(err)
		}
		expectedSHA := meta["sha256"].(string)

		// Step 2: Download binary
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agent/binary?arch=amd64", nil)
		w = httptest.NewRecorder()
		deps.HandleAgentBinary(w, req)

		// Step 3: Compute SHA256 of downloaded content
		h := sha256.New()
		if _, err := io.Copy(h, w.Body); err != nil {
			t.Fatal(err)
		}
		actualSHA := hex.EncodeToString(h.Sum(nil))

		// Step 4: Verify they match
		if actualSHA != expectedSHA {
			t.Errorf("SHA256 mismatch: downloaded=%s, metadata=%s", actualSHA, expectedSHA)
		}
	})
}

// Ensure servicehttp is used (avoids unused import if tests change).
var _ = servicehttp.WriteJSON
