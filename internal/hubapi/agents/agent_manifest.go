package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ManifestFilename = "agent-manifest.json"

// AgentManifest describes the set of versioned agent binaries available for
// distribution. It is loaded from an agent-manifest.json file written by the
// release pipeline.
type AgentManifest struct {
	SchemaVersion int                   `json:"schema_version"`
	GeneratedAt   string                `json:"generated_at"`
	HubVersion    string                `json:"hub_version"`
	Agents        map[string]AgentEntry `json:"agents"`
}

// AgentEntry holds the release metadata for a single agent type.
type AgentEntry struct {
	Version  string                 `json:"version"`
	Repo     string                 `json:"repo"`
	Type     string                 `json:"type,omitempty"`
	Binaries map[string]BinaryEntry `json:"binaries"`
}

// BinaryEntry describes a single platform binary artifact.
type BinaryEntry struct {
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	URL       string `json:"url"`
}

// LoadAgentManifest reads and parses the agent-manifest.json file from dir.
func LoadAgentManifest(dir string) (*AgentManifest, error) {
	path := filepath.Join(dir, ManifestFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read agent manifest: %w", err)
	}
	var m AgentManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse agent manifest: %w", err)
	}
	return &m, nil
}

// LookupBinary returns the BinaryEntry for the Go agent on the given OS and
// architecture. The key format is "<os>-<arch>" (e.g. "linux-amd64").
func (m *AgentManifest) LookupBinary(agentOS, arch string) (*BinaryEntry, error) {
	agent, ok := m.Agents["labtether-agent"]
	if !ok {
		return nil, fmt.Errorf("no labtether-agent entry in manifest")
	}
	key := strings.ToLower(agentOS) + "-" + strings.ToLower(arch)
	bin, ok := agent.Binaries[key]
	if !ok {
		return nil, fmt.Errorf("no binary for %s", key)
	}
	return &bin, nil
}

// GoAgentVersion returns the version string for the labtether-agent entry, or
// an empty string if the entry is absent.
func (m *AgentManifest) GoAgentVersion() string {
	if agent, ok := m.Agents["labtether-agent"]; ok {
		return agent.Version
	}
	return ""
}
