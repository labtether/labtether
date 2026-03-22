package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// ResolveAgentBinaryDir returns the directory that contains pre-built agent
// binaries. It checks, in order:
//  1. LABTETHER_AGENT_DIR environment variable
//  2. ./build/   (development — binaries built locally)
//  3. /opt/labtether/agents/  (production Docker image)
func ResolveAgentBinaryDir() string {
	if dir := strings.TrimSpace(os.Getenv("LABTETHER_AGENT_DIR")); dir != "" {
		return dir
	}

	if info, err := os.Stat("./build"); err == nil && info.IsDir() {
		return "./build"
	}

	return "/opt/labtether/agents"
}

// handleAgentBinary serves the pre-built labtether-agent binary for the
// requested architecture. The endpoint is intentionally public (no auth)
// so that the install script can download the binary without credentials.
//
// Query parameters:
//
//	arch — required; "amd64" or "arm64"
func (d *Deps) HandleAgentBinary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentOS := strings.TrimSpace(r.URL.Query().Get("os"))
	if agentOS == "" {
		agentOS = "linux"
	}
	arch := strings.TrimSpace(r.URL.Query().Get("arch"))
	if arch == "" {
		http.Error(w, "arch query parameter is required", http.StatusBadRequest)
		return
	}
	binaryName, binaryPath, err := ResolveAgentBinaryPath(d.AgentBinaryDir, agentOS, arch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// #nosec G304,G703 -- binaryPath is constrained by ResolveAgentBinaryPath allowlist.
	f, err := os.Open(binaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "agent binary not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to open agent binary", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "failed to stat agent binary", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, binaryName))
	http.ServeContent(w, r, binaryName, info.ModTime(), f)
}

// handleAgentReleaseLatest returns metadata for the latest available agent binary.
// Query parameters:
//
//	os   — optional; defaults to "linux"
//	arch — required for linux/windows; optional for darwin (accepted but ignored)
func (d *Deps) HandleAgentReleaseLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentOS := strings.TrimSpace(r.URL.Query().Get("os"))
	if agentOS == "" {
		agentOS = "linux"
	}
	arch := strings.TrimSpace(r.URL.Query().Get("arch"))
	if arch == "" {
		http.Error(w, "arch query parameter is required", http.StatusBadRequest)
		return
	}

	binaryName, binaryPath, err := ResolveAgentBinaryPath(d.AgentBinaryDir, agentOS, arch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// #nosec G304,G703 -- binaryPath is constrained by ResolveAgentBinaryPath allowlist.
	f, err := os.Open(binaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "agent binary not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to open agent binary", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "failed to stat agent binary", http.StatusInternalServerError)
		return
	}

	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		http.Error(w, "failed to hash agent binary", http.StatusInternalServerError)
		return
	}
	sha := hex.EncodeToString(sum.Sum(nil))

	hubURL := d.ResolveHubURL(r)
	binaryURL := fmt.Sprintf(
		"%s/api/v1/agent/binary?os=%s&arch=%s",
		hubURL,
		url.QueryEscape(agentOS),
		url.QueryEscape(arch),
	)

	version := strings.TrimSpace(shared.EnvOrDefault("LABTETHER_AGENT_RELEASE_VERSION", ""))
	if version == "" {
		version = "sha256:" + sha[:12]
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"version":      version,
		"os":           agentOS,
		"arch":         arch,
		"binary_name":  binaryName,
		"size_bytes":   info.Size(),
		"sha256":       sha,
		"url":          binaryURL,
		"published_at": info.ModTime().UTC().Format(time.RFC3339),
	})
}

func ResolveAgentBinaryPath(binaryDir, agentOS, arch string) (string, string, error) {
	agentOS = strings.ToLower(strings.TrimSpace(agentOS))
	arch = strings.ToLower(strings.TrimSpace(arch))

	switch agentOS {
	case "linux":
		if arch != "amd64" && arch != "arm64" {
			return "", "", fmt.Errorf("arch must be amd64 or arm64")
		}
		name := fmt.Sprintf("labtether-agent-linux-%s", arch)
		return name, filepath.Join(binaryDir, name), nil
	case "darwin":
		// Current macOS packaging publishes a universal binary.
		name := "labtether-agent-darwin"
		return name, filepath.Join(binaryDir, name), nil
	case "windows":
		if arch != "amd64" && arch != "arm64" {
			return "", "", fmt.Errorf("arch must be amd64 or arm64")
		}
		name := fmt.Sprintf("labtether-agent-windows-%s.exe", arch)
		return name, filepath.Join(binaryDir, name), nil
	default:
		return "", "", fmt.Errorf("os must be linux, darwin, or windows")
	}
}
