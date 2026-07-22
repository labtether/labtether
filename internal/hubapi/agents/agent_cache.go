package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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

// OpenVerifiedBinary opens and verifies a manifest entry without ever serving
// a same-named artifact from another release. Persistent runtime caches survive
// hub image upgrades, so filename precedence alone is insufficient: an older
// cached binary can otherwise override the newer manifest baked into the
// image. The already-verified file descriptor is returned to the caller so a
// writable runtime-cache path cannot be swapped between verification and
// response streaming.
func (c *AgentCache) OpenVerifiedBinary(binaryName, expectedSHA256 string, expectedSize int64) (*os.File, os.FileInfo, error) {
	if strings.Contains(binaryName, "/") || strings.Contains(binaryName, "\\") || strings.Contains(binaryName, "..") {
		return nil, nil, fmt.Errorf("invalid binary name: %q", binaryName)
	}
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	if len(expectedSHA256) != sha256.Size*2 {
		return nil, nil, fmt.Errorf("agent binary %q has invalid manifest sha256", binaryName)
	}
	if _, err := hex.DecodeString(expectedSHA256); err != nil {
		return nil, nil, fmt.Errorf("agent binary %q has invalid manifest sha256", binaryName)
	}

	for _, dir := range []string{c.RuntimeDir, c.BakedInDir} {
		path := filepath.Join(dir, binaryName)
		file, info, matches, err := openBinaryMatchingManifest(path, expectedSHA256, expectedSize)
		if err != nil {
			continue
		}
		if matches {
			return file, info, nil
		}
		_ = file.Close()
	}
	return nil, nil, fmt.Errorf("agent binary %q does not match the active manifest", binaryName)
}

func openBinaryMatchingManifest(path, expectedSHA256 string, expectedSize int64) (*os.File, os.FileInfo, bool, error) {
	file, err := os.Open(path) // #nosec G304 -- caller joins a separator-free manifest filename to a configured cache root.
	if err != nil {
		return nil, nil, false, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, false, err
	}
	if !info.Mode().IsRegular() || (expectedSize > 0 && info.Size() != expectedSize) {
		return file, info, false, nil
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		_ = file.Close()
		return nil, nil, false, err
	}
	if hex.EncodeToString(hasher.Sum(nil)) != expectedSHA256 {
		return file, info, false, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, nil, false, err
	}
	return file, info, true, nil
}

// Manifest returns the currently loaded AgentManifest. May be nil if no
// manifest has been loaded yet.
func (c *AgentCache) Manifest() *AgentManifest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.manifest
}

// LoadManifest selects the newest readable manifest across the persistent
// runtime cache and the current image. This lets a deliberate runtime refresh
// supersede the image while preventing an older cache volume from silently
// downgrading a newly deployed hub.
func (c *AgentCache) LoadManifest() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	runtimeManifest, runtimeErr := LoadAgentManifest(c.RuntimeDir)
	bakedManifest, bakedErr := LoadAgentManifest(c.BakedInDir)
	switch {
	case runtimeErr == nil && bakedErr == nil:
		if manifestGeneratedAfter(runtimeManifest, bakedManifest) {
			c.manifest = runtimeManifest
		} else {
			c.manifest = bakedManifest
		}
	case runtimeErr == nil:
		c.manifest = runtimeManifest
	case bakedErr == nil:
		c.manifest = bakedManifest
	default:
		return fmt.Errorf("load agent manifest: runtime: %v; baked-in: %w", runtimeErr, bakedErr)
	}
	return nil
}

func manifestGeneratedAfter(candidate, baseline *AgentManifest) bool {
	candidateTime, candidateErr := time.Parse(time.RFC3339, strings.TrimSpace(candidate.GeneratedAt))
	baselineTime, baselineErr := time.Parse(time.RFC3339, strings.TrimSpace(baseline.GeneratedAt))
	if candidateErr == nil && baselineErr == nil {
		return candidateTime.After(baselineTime)
	}
	if candidateErr == nil {
		return true
	}
	if baselineErr == nil {
		return false
	}
	// Preserve the historical runtime preference only when freshness cannot be
	// compared. Release-generated manifests always carry RFC3339 timestamps.
	return true
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
