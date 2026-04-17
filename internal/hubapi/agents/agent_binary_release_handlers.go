package agents

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/labtether/labtether/internal/servicehttp"
)

// HandleAgentBinary serves the pre-built labtether-agent binary for the
// requested architecture. The endpoint is intentionally public (no auth)
// so that the install script can download the binary without credentials.
//
// Binary lookup is driven by the agent manifest loaded in AgentCache.
//
// Query parameters:
//
//	os   — optional; defaults to "linux"
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

	m := d.AgentCache.Manifest()
	if m == nil {
		http.Error(w, "agent manifest not loaded", http.StatusServiceUnavailable)
		return
	}

	bin, err := m.LookupBinary(agentOS, arch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	binaryPath, err := d.AgentCache.ResolveBinaryPath(bin.Name)
	if err != nil {
		http.Error(w, "agent binary not found", http.StatusNotFound)
		return
	}

	// #nosec G304 -- binaryPath is constrained by AgentCache.ResolveBinaryPath path-traversal check.
	f, err := os.Open(binaryPath)
	if err != nil {
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
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, bin.Name))
	http.ServeContent(w, r, bin.Name, info.ModTime(), f)
}

// HandleAgentReleaseLatest returns metadata for the latest available agent binary.
// Release metadata (version, sha256, size) is read from the agent manifest
// instead of being computed at request time.
//
// Query parameters:
//
//	os   — optional; defaults to "linux"
//	arch — required; "amd64" or "arm64"
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

	m := d.AgentCache.Manifest()
	if m == nil {
		http.Error(w, "agent manifest not loaded", http.StatusServiceUnavailable)
		return
	}

	bin, err := m.LookupBinary(agentOS, arch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hubURL := d.ResolveHubURL(r)
	binaryURL := fmt.Sprintf(
		"%s/api/v1/agent/binary?os=%s&arch=%s",
		hubURL,
		url.QueryEscape(agentOS),
		url.QueryEscape(arch),
	)

	version := m.GoAgentVersion()
	if version == "" {
		version = "unknown"
	}

	response := map[string]any{
		"version":      version,
		"os":           agentOS,
		"arch":         arch,
		"binary_name":  bin.Name,
		"size_bytes":   bin.SizeBytes,
		"sha256":       bin.SHA256,
		"url":          binaryURL,
		"published_at": m.GeneratedAt,
	}
	if bin.Signature != "" {
		response["signature"] = bin.Signature
	}
	servicehttp.WriteJSON(w, http.StatusOK, response)
}
