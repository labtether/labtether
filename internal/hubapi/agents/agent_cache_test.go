package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeAgentCacheManifest(t *testing.T, dir, generatedAt, hubVersion string) {
	t.Helper()
	payload, err := json.Marshal(AgentManifest{
		SchemaVersion: 1,
		GeneratedAt:   generatedAt,
		HubVersion:    hubVersion,
		Agents:        map[string]AgentEntry{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFilename), payload, 0600); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func TestAgentCache_ResolveBinaryPath_RuntimeCacheFirst(t *testing.T) {
	runtime := t.TempDir()
	bakedIn := t.TempDir()
	if err := os.WriteFile(filepath.Join(runtime, "labtether-agent-linux-amd64"), []byte("runtime"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bakedIn, "labtether-agent-linux-amd64"), []byte("baked"), 0755); err != nil {
		t.Fatal(err)
	}
	cache := &AgentCache{RuntimeDir: runtime, BakedInDir: bakedIn}
	path, err := cache.ResolveBinaryPath("labtether-agent-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "runtime" {
		t.Errorf("expected runtime binary, got %q", string(data))
	}
}

func TestAgentCache_ResolveBinaryPath_FallbackToBakedIn(t *testing.T) {
	runtime := t.TempDir()
	bakedIn := t.TempDir()
	if err := os.WriteFile(filepath.Join(bakedIn, "labtether-agent-linux-amd64"), []byte("baked"), 0755); err != nil {
		t.Fatal(err)
	}
	cache := &AgentCache{RuntimeDir: runtime, BakedInDir: bakedIn}
	path, err := cache.ResolveBinaryPath("labtether-agent-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "baked" {
		t.Errorf("expected baked-in binary, got %q", string(data))
	}
}

func TestAgentCache_ResolveBinaryPath_NotFound(t *testing.T) {
	cache := &AgentCache{RuntimeDir: t.TempDir(), BakedInDir: t.TempDir()}
	_, err := cache.ResolveBinaryPath("labtether-agent-linux-amd64")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestAgentCache_ResolveBinaryPath_RejectsPathTraversal(t *testing.T) {
	cache := &AgentCache{RuntimeDir: t.TempDir(), BakedInDir: t.TempDir()}
	_, err := cache.ResolveBinaryPath("../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestAgentCache_LoadManifestPrefersNewerBakedImageOverStaleRuntimeCache(t *testing.T) {
	runtime := t.TempDir()
	bakedIn := t.TempDir()
	writeAgentCacheManifest(t, runtime, "2026-07-13T22:31:00Z", "qa-r8")
	writeAgentCacheManifest(t, bakedIn, "2026-07-14T06:15:21Z", "qa-r9")

	cache := &AgentCache{RuntimeDir: runtime, BakedInDir: bakedIn}
	if err := cache.LoadManifest(); err != nil {
		t.Fatal(err)
	}
	if got := cache.Manifest().HubVersion; got != "qa-r9" {
		t.Fatalf("hub version = %q, want qa-r9", got)
	}
}

func TestAgentCache_LoadManifestAllowsNewerRuntimeRefresh(t *testing.T) {
	runtime := t.TempDir()
	bakedIn := t.TempDir()
	writeAgentCacheManifest(t, runtime, "2026-07-15T00:00:00Z", "qa-r10")
	writeAgentCacheManifest(t, bakedIn, "2026-07-14T06:15:21Z", "qa-r9")

	cache := &AgentCache{RuntimeDir: runtime, BakedInDir: bakedIn}
	if err := cache.LoadManifest(); err != nil {
		t.Fatal(err)
	}
	if got := cache.Manifest().HubVersion; got != "qa-r10" {
		t.Fatalf("hub version = %q, want qa-r10", got)
	}
}

func TestAgentCache_LoadManifestPrefersValidTimestamp(t *testing.T) {
	runtime := t.TempDir()
	bakedIn := t.TempDir()
	writeAgentCacheManifest(t, runtime, "invalid", "stale-runtime")
	writeAgentCacheManifest(t, bakedIn, "2026-07-14T06:15:21Z", "valid-baked")

	cache := &AgentCache{RuntimeDir: runtime, BakedInDir: bakedIn}
	if err := cache.LoadManifest(); err != nil {
		t.Fatal(err)
	}
	if got := cache.Manifest().HubVersion; got != "valid-baked" {
		t.Fatalf("hub version = %q, want valid-baked", got)
	}
}

func TestAgentCache_OpenVerifiedBinarySkipsStaleRuntimeArtifact(t *testing.T) {
	runtime := t.TempDir()
	bakedIn := t.TempDir()
	name := "labtether-agent-linux-amd64"
	if err := os.WriteFile(filepath.Join(runtime, name), []byte("old-runtime"), 0755); err != nil {
		t.Fatal(err)
	}
	baked := []byte("new-baked")
	if err := os.WriteFile(filepath.Join(bakedIn, name), baked, 0755); err != nil {
		t.Fatal(err)
	}

	cache := &AgentCache{RuntimeDir: runtime, BakedInDir: bakedIn}
	file, _, err := cache.OpenVerifiedBinary(name, sha256Hex(baked), int64(len(baked)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if file.Name() != filepath.Join(bakedIn, name) {
		t.Fatalf("path = %q, want baked-in artifact", file.Name())
	}
}

func TestAgentCache_OpenVerifiedBinaryFailsClosedOnMismatch(t *testing.T) {
	runtime := t.TempDir()
	bakedIn := t.TempDir()
	name := "labtether-agent-linux-amd64"
	if err := os.WriteFile(filepath.Join(runtime, name), []byte("old-runtime"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bakedIn, name), []byte("old-baked"), 0755); err != nil {
		t.Fatal(err)
	}

	cache := &AgentCache{RuntimeDir: runtime, BakedInDir: bakedIn}
	if _, _, err := cache.OpenVerifiedBinary(name, sha256Hex([]byte("expected")), int64(len("expected"))); err == nil {
		t.Fatal("expected mismatched artifacts to fail closed")
	}
}

func TestAgentCache_OpenVerifiedBinaryStreamsTheVerifiedDescriptorAfterPathSwap(t *testing.T) {
	runtimeDir := t.TempDir()
	bakedIn := t.TempDir()
	name := "labtether-agent-linux-amd64"
	trusted := []byte("trusted-release-binary")
	path := filepath.Join(runtimeDir, name)
	if err := os.WriteFile(path, trusted, 0755); err != nil {
		t.Fatal(err)
	}

	cache := &AgentCache{RuntimeDir: runtimeDir, BakedInDir: bakedIn}
	file, _, err := cache.OpenVerifiedBinary(name, sha256Hex(trusted), int64(len(trusted)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	malicious := filepath.Join(runtimeDir, "replacement")
	if err := os.WriteFile(malicious, []byte("unverified-replacement"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(malicious, path); err != nil {
		t.Fatal(err)
	}

	served, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(served) != string(trusted) {
		t.Fatalf("verified descriptor served %q, want trusted artifact", served)
	}
}

func TestAgentCache_VerifiedBinaryContentKeepsVerifiedBytes(t *testing.T) {
	runtimeDir := t.TempDir()
	name := "labtether-agent-linux-amd64"
	trusted := []byte("trusted-release-binary")
	path := filepath.Join(runtimeDir, name)
	if err := os.WriteFile(path, trusted, 0o755); err != nil {
		t.Fatal(err)
	}
	cache := &AgentCache{RuntimeDir: runtimeDir, BakedInDir: t.TempDir()}
	digest := sha256Hex(trusted)
	got, _, err := cache.VerifiedBinaryContent(name, digest, int64(len(trusted)))
	if err != nil || string(got) != string(trusted) {
		t.Fatalf("first verified content = %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "replacement"), []byte("untrusted bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(runtimeDir, "replacement"), path); err != nil {
		t.Fatal(err)
	}
	got, _, err = cache.VerifiedBinaryContent(name, digest, int64(len(trusted)))
	if err != nil || string(got) != string(trusted) {
		t.Fatalf("cached verified bytes changed after path swap: %q, %v", got, err)
	}

	cache.SetManifest(&AgentManifest{}) // a refresh invalidates both verdicts
	if _, _, err := cache.VerifiedBinaryContent(name, digest, int64(len(trusted))); err == nil {
		t.Fatal("unverified replacement was accepted after refresh")
	}
	if err := os.WriteFile(path, trusted, 0o755); err != nil {
		t.Fatal(err)
	}
	got, _, err = cache.VerifiedBinaryContent(name, digest, int64(len(trusted)))
	if err != nil || string(got) != string(trusted) {
		t.Fatalf("replacement with valid content did not clear failed verdict: %q, %v", got, err)
	}
}

func TestAgentCache_VerifiedBinaryContentAcceptsReleaseSizeAbove64MiB(t *testing.T) {
	runtimeDir := t.TempDir()
	name := "labtether-agent-linux-amd64"
	data := make([]byte, (64<<20)+1)
	if err := os.WriteFile(filepath.Join(runtimeDir, name), data, 0o755); err != nil {
		t.Fatal(err)
	}
	cache := &AgentCache{RuntimeDir: runtimeDir, BakedInDir: t.TempDir()}
	got, _, err := cache.VerifiedBinaryContent(name, sha256Hex(data), int64(len(data)))
	if err != nil || len(got) != len(data) {
		t.Fatalf("valid release size was rejected: bytes=%d, err=%v", len(got), err)
	}
}

func TestAgentCache_VerifiedBinaryContentRechecksPreservedTimeRepair(t *testing.T) {
	runtimeDir := t.TempDir()
	name := "labtether-agent-linux-amd64"
	path := filepath.Join(runtimeDir, name)
	trusted := []byte("trusted")
	if err := os.WriteFile(path, []byte("invalid"), 0o755); err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	cache := &AgentCache{RuntimeDir: runtimeDir, BakedInDir: t.TempDir()}
	if _, _, err := cache.VerifiedBinaryContent(name, sha256Hex(trusted), int64(len(trusted))); err == nil {
		t.Fatal("invalid bytes were accepted")
	}
	time.Sleep(10 * time.Millisecond) // make the metadata change observable on CI filesystems
	if err := os.WriteFile(path, trusted, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, original.ModTime(), original.ModTime()); err != nil {
		t.Fatal(err)
	}
	got, _, err := cache.VerifiedBinaryContent(name, sha256Hex(trusted), int64(len(trusted)))
	if err != nil || string(got) != string(trusted) {
		t.Fatalf("repaired binary was not served: %q, %v", got, err)
	}
}

func TestHandleAgentBinaryRangeUsesVerifiedSnapshot(t *testing.T) {
	runtimeDir := t.TempDir()
	name := "labtether-agent-linux-amd64"
	trusted := []byte("trusted-release-binary")
	path := filepath.Join(runtimeDir, name)
	if err := os.WriteFile(path, trusted, 0o755); err != nil {
		t.Fatal(err)
	}
	cache := &AgentCache{RuntimeDir: runtimeDir, BakedInDir: t.TempDir()}
	cache.SetManifest(&AgentManifest{Agents: map[string]AgentEntry{
		"labtether-agent": {Binaries: map[string]BinaryEntry{
			"linux-amd64": {Name: name, SHA256: sha256Hex(trusted), SizeBytes: int64(len(trusted))},
		}},
	}})
	deps := &Deps{AgentCache: cache}
	for _, tc := range []struct {
		rangeHeader string
		want        string
	}{
		{"bytes=0-0", "t"},
		{"bytes=1-1", "r"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/binary?arch=amd64", nil)
		req.Header.Set("Range", tc.rangeHeader)
		response := httptest.NewRecorder()
		deps.HandleAgentBinary(response, req)
		if response.Code != http.StatusPartialContent || response.Body.String() != tc.want {
			t.Fatalf("range %s = %d %q, want 206 %q", tc.rangeHeader, response.Code, response.Body.String(), tc.want)
		}
		if tc.rangeHeader == "bytes=0-0" {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
	}
}
