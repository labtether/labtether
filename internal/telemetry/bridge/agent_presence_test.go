package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockAgentPresenceSource implements AgentPresenceSource for testing.
type mockAgentPresenceSource struct {
	entries []AgentPresenceEntry
}

func (m *mockAgentPresenceSource) AllAgentPresenceMetrics() []AgentPresenceEntry {
	return m.entries
}

func TestAgentPresenceBridgeCollect(t *testing.T) {
	source := &mockAgentPresenceSource{
		entries: []AgentPresenceEntry{
			{
				AssetID:             "asset-linux-server-01",
				Connected:           1,
				LastHeartbeatAgeSec: 12.0,
				Labels: map[string]string{
					"agent_version": "1.4.2",
				},
			},
			{
				AssetID:             "asset-linux-server-02",
				Connected:           0,
				LastHeartbeatAgeSec: 420.0,
				Labels: map[string]string{
					"agent_version": "1.3.9",
				},
			},
		},
	}

	b := NewAgentPresenceBridge(source)

	if b.Name() != "agent-presence" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 4 {
		t.Fatalf("expected 4 samples (2 per agent), got %d", len(samples))
	}

	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		byKey[s.AssetID+":"+s.Metric] = s
	}

	// --- Agent 1: connected ---
	a1 := "asset-linux-server-01"
	assertSample(t, byKey, a1, telemetry.MetricAgentConnected, "bool", 1)
	assertSample(t, byKey, a1, telemetry.MetricAgentLastHeartbeatAgeSeconds, "seconds", 12.0)

	// --- Agent 2: disconnected ---
	a2 := "asset-linux-server-02"
	assertSample(t, byKey, a2, telemetry.MetricAgentConnected, "bool", 0)
	assertSample(t, byKey, a2, telemetry.MetricAgentLastHeartbeatAgeSeconds, "seconds", 420.0)
}

func TestAgentPresenceBridgeLabels(t *testing.T) {
	wantLabels := map[string]string{
		"agent_version": "1.5.0",
	}

	source := &mockAgentPresenceSource{
		entries: []AgentPresenceEntry{
			{
				AssetID:             "asset-mac-workstation",
				Connected:           1,
				LastHeartbeatAgeSec: 5.0,
				Labels:              wantLabels,
			},
		},
	}

	b := NewAgentPresenceBridge(source)
	samples := b.Collect()

	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}

	for _, s := range samples {
		if s.Labels == nil {
			t.Errorf("metric %q: expected labels, got nil", s.Metric)
			continue
		}
		for k, want := range wantLabels {
			got, ok := s.Labels[k]
			if !ok {
				t.Errorf("metric %q: missing label %q", s.Metric, k)
				continue
			}
			if got != want {
				t.Errorf("metric %q label %q: got %q, want %q", s.Metric, k, got, want)
			}
		}
	}
}

func TestAgentPresenceBridgeEmpty(t *testing.T) {
	source := &mockAgentPresenceSource{entries: nil}
	b := NewAgentPresenceBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}
