package agents

import (
	"os"
	"path/filepath"
	"testing"
)

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
