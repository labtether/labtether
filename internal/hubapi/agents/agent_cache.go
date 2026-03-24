package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AgentCache resolves agent binaries using a two-tier lookup: the runtime
// directory (persistent volume, writable) is checked first, and the baked-in
// directory (Docker image, read-only) is used as a fallback. It also holds a
// thread-safe reference to the current AgentManifest and enforces a cooldown
// between manifest refresh attempts.
type AgentCache struct {
	RuntimeDir string // /data/agents/ — persistent volume, writable
	BakedInDir string // /opt/labtether/agents/ — baked into image, read-only

	mu              sync.Mutex
	manifest        *AgentManifest
	lastRefresh     time.Time
	refreshCooldown time.Duration // default 5 * time.Minute
}

// ResolveBinaryPath returns the absolute path to the named binary. It checks
// RuntimeDir first, then BakedInDir. Returns an error if the name contains
// path separators or ".." (path-traversal protection), or if the binary is not
// present in either directory.
func (c *AgentCache) ResolveBinaryPath(binaryName string) (string, error) {
	if strings.Contains(binaryName, "/") || strings.Contains(binaryName, "\\") || strings.Contains(binaryName, "..") {
		return "", fmt.Errorf("invalid binary name: %q", binaryName)
	}
	runtimePath := filepath.Join(c.RuntimeDir, binaryName)
	if _, err := os.Stat(runtimePath); err == nil {
		return runtimePath, nil
	}
	bakedPath := filepath.Join(c.BakedInDir, binaryName)
	if _, err := os.Stat(bakedPath); err == nil {
		return bakedPath, nil
	}
	return "", fmt.Errorf("agent binary %q not found in cache or baked-in directory", binaryName)
}

// Manifest returns the currently loaded AgentManifest. May be nil if no
// manifest has been loaded yet.
func (c *AgentCache) Manifest() *AgentManifest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.manifest
}

// LoadManifest reads agent-manifest.json from RuntimeDir, falling back to
// BakedInDir if the runtime copy is absent or unreadable.
func (c *AgentCache) LoadManifest() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if m, err := LoadAgentManifest(c.RuntimeDir); err == nil {
		c.manifest = m
		return nil
	}
	m, err := LoadAgentManifest(c.BakedInDir)
	if err != nil {
		return fmt.Errorf("load agent manifest: %w", err)
	}
	c.manifest = m
	return nil
}

// SetManifest replaces the in-memory manifest (used after a successful
// download and write of a new manifest to RuntimeDir).
func (c *AgentCache) SetManifest(m *AgentManifest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manifest = m
}

// TryRefresh records a refresh attempt and returns true if enough time has
// elapsed since the last attempt. Returns false if the caller should wait.
// The zero value of refreshCooldown defaults to 5 minutes.
func (c *AgentCache) TryRefresh() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	cooldown := c.refreshCooldown
	if cooldown == 0 {
		cooldown = 5 * time.Minute
	}
	if time.Since(c.lastRefresh) < cooldown {
		return false
	}
	c.lastRefresh = time.Now()
	return true
}
