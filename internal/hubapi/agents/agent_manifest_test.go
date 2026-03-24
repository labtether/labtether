package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAgentManifest_ValidFile(t *testing.T) {
	dir := t.TempDir()
	data := `{
		"schema_version": 1,
		"generated_at": "2026-03-24T12:00:00Z",
		"hub_version": "v2026.1",
		"agents": {
			"labtether-agent": {
				"version": "v2026.1",
				"repo": "labtether/labtether-agent",
				"binaries": {
					"linux-amd64": {
						"name": "labtether-agent-linux-amd64",
						"sha256": "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
						"size_bytes": 18900000,
						"url": "https://github.com/labtether/labtether-agent/releases/download/v2026.1/labtether-agent-linux-amd64"
					}
				}
			},
			"labtether-mac": {
				"version": "v2026.1",
				"repo": "labtether/labtether-mac",
				"type": "metadata-only",
				"binaries": {
					"darwin-universal": {
						"name": "labtether-agent-macos-universal.tar.gz",
						"sha256": "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
						"url": "https://github.com/labtether/labtether-mac/releases/download/v2026.1/labtether-agent-macos-universal.tar.gz"
					}
				}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "agent-manifest.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadAgentManifest(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", m.SchemaVersion)
	}
	if m.HubVersion != "v2026.1" {
		t.Errorf("hub_version = %q, want %q", m.HubVersion, "v2026.1")
	}
	agent, ok := m.Agents["labtether-agent"]
	if !ok {
		t.Fatal("missing labtether-agent entry")
	}
	if agent.Version != "v2026.1" {
		t.Errorf("agent version = %q, want %q", agent.Version, "v2026.1")
	}
	if agent.Type != "" {
		t.Errorf("agent type = %q, want empty", agent.Type)
	}
	bin, ok := agent.Binaries["linux-amd64"]
	if !ok {
		t.Fatal("missing linux-amd64 binary entry")
	}
	if bin.Name != "labtether-agent-linux-amd64" {
		t.Errorf("binary name = %q", bin.Name)
	}

	mac, ok := m.Agents["labtether-mac"]
	if !ok {
		t.Fatal("missing labtether-mac entry")
	}
	if mac.Type != "metadata-only" {
		t.Errorf("mac type = %q, want %q", mac.Type, "metadata-only")
	}
}

func TestLoadAgentManifest_MissingFile(t *testing.T) {
	_, err := LoadAgentManifest(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestAgentManifest_LookupBinary(t *testing.T) {
	m := &AgentManifest{
		Agents: map[string]AgentEntry{
			"labtether-agent": {
				Version: "v2026.1",
				Binaries: map[string]BinaryEntry{
					"linux-amd64": {Name: "labtether-agent-linux-amd64", SHA256: "abc123"},
				},
			},
		},
	}
	bin, err := m.LookupBinary("linux", "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bin.Name != "labtether-agent-linux-amd64" {
		t.Errorf("name = %q", bin.Name)
	}

	_, err = m.LookupBinary("freebsd", "amd64")
	if err == nil {
		t.Fatal("expected error for unknown platform")
	}
}

func TestAgentManifest_GoAgentVersion(t *testing.T) {
	m := &AgentManifest{
		Agents: map[string]AgentEntry{
			"labtether-agent": {Version: "v2026.1"},
		},
	}
	if v := m.GoAgentVersion(); v != "v2026.1" {
		t.Errorf("GoAgentVersion() = %q, want %q", v, "v2026.1")
	}
}
