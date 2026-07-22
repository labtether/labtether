package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
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
