package agentcore

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

type recordingHeartbeatPublisher struct {
	mu      sync.Mutex
	samples []TelemetrySample
	err     error
}

func (p *recordingHeartbeatPublisher) Publish(_ context.Context, sample TelemetrySample) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.samples = append(p.samples, sample)
	return p.err
}

func (p *recordingHeartbeatPublisher) snapshot() []TelemetrySample {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]TelemetrySample, len(p.samples))
	copy(out, p.samples)
	return out
}

func TestRuntimePublishOnceBuffersTelemetryWhenDisconnected(t *testing.T) {
	publisher := &recordingHeartbeatPublisher{}
	runtime := NewRuntime(RuntimeConfig{AssetID: "node-1"}, nil, publisher)
	runtime.transport = &wsTransport{}
	runtime.telemetryBuf = NewRingBuffer[TelemetrySample](4)

	now := time.Now().UTC()
	runtime.mu.Lock()
	runtime.sample = TelemetrySample{
		AssetID:       "node-1",
		CPUPercent:    17,
		MemoryPercent: 33,
		CollectedAt:   now,
	}
	runtime.mu.Unlock()

	runtime.publishOnce(context.Background())

	published := publisher.snapshot()
	if len(published) != 1 {
		t.Fatalf("expected one publisher call, got %d", len(published))
	}
	if published[0].AssetID != "node-1" {
		t.Fatalf("publisher asset_id=%q, want node-1", published[0].AssetID)
	}

	buffered := runtime.telemetryBuf.Drain()
	if len(buffered) != 1 {
		t.Fatalf("expected one buffered telemetry sample, got %d", len(buffered))
	}
	if buffered[0].AssetID != "node-1" || buffered[0].CPUPercent != 17 || buffered[0].MemoryPercent != 33 {
		t.Fatalf("unexpected buffered sample: %+v", buffered[0])
	}
}

func TestReplayBufferedTelemetrySendsSamplesInOrder(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()
	transport.connected = true

	telemetryBuf := NewRingBuffer[TelemetrySample](4)
	telemetryBuf.Push(TelemetrySample{
		AssetID:       "node-1",
		CPUPercent:    11,
		MemoryPercent: 21,
		DiskPercent:   31,
	})
	telemetryBuf.Push(TelemetrySample{
		AssetID:       "node-1",
		CPUPercent:    12,
		MemoryPercent: 22,
		DiskPercent:   32,
	})

	replayBufferedTelemetry(transport, telemetryBuf)

	if got := telemetryBuf.Len(); got != 0 {
		t.Fatalf("expected replay buffer to be drained, got len=%d", got)
	}

	first := waitForCapturedAgentMessage(t, messages, agentmgr.MsgTelemetry, 2*time.Second)
	second := waitForCapturedAgentMessage(t, messages, agentmgr.MsgTelemetry, 2*time.Second)

	var firstPayload agentmgr.TelemetryData
	if err := json.Unmarshal(first.Data, &firstPayload); err != nil {
		t.Fatalf("decode first telemetry payload: %v", err)
	}
	var secondPayload agentmgr.TelemetryData
	if err := json.Unmarshal(second.Data, &secondPayload); err != nil {
		t.Fatalf("decode second telemetry payload: %v", err)
	}

	if firstPayload.AssetID != "node-1" || firstPayload.CPUPercent != 11 || firstPayload.MemoryPercent != 21 || firstPayload.DiskPercent != 31 {
		t.Fatalf("unexpected first replayed payload: %+v", firstPayload)
	}
	if secondPayload.AssetID != "node-1" || secondPayload.CPUPercent != 12 || secondPayload.MemoryPercent != 22 || secondPayload.DiskPercent != 32 {
		t.Fatalf("unexpected second replayed payload: %+v", secondPayload)
	}
}

func TestWSHeartbeatPublisherFallsBackToHTTPWhenDisconnected(t *testing.T) {
	fallback := &recordingHeartbeatPublisher{}
	publisher := newWSHeartbeatPublisher(&wsTransport{}, fallback, RuntimeConfig{
		Source:  "agent",
		GroupID: "group-1",
	}, map[string]string{
		"platform": "linux",
	}, []string{"terminal"})

	if err := publisher.Publish(context.Background(), TelemetrySample{AssetID: "node-1"}); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	published := fallback.snapshot()
	if len(published) != 1 {
		t.Fatalf("expected one fallback publish, got %d", len(published))
	}
	if published[0].AssetID != "node-1" {
		t.Fatalf("fallback asset_id=%q, want node-1", published[0].AssetID)
	}
}

func TestWSHeartbeatPublisherSendsHeartbeatWhenConnected(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()
	transport.connected = true
	transport.startedAt = time.Now().Add(-3 * time.Second)

	fallback := &recordingHeartbeatPublisher{}
	publisher := newWSHeartbeatPublisher(transport, fallback, RuntimeConfig{
		Source:  "agent",
		GroupID: "group-1",
	}, map[string]string{
		"os_name": "Ubuntu 24.04 LTS",
	}, []string{"terminal", "files"})

	if err := publisher.Publish(context.Background(), TelemetrySample{
		AssetID:          "node-1",
		CPUPercent:       41,
		MemoryPercent:    52,
		DiskPercent:      63,
		NetRXBytesPerSec: 74,
		NetTXBytesPerSec: 85,
	}); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgHeartbeat, 2*time.Second)
	var heartbeat agentmgr.HeartbeatData
	if err := json.Unmarshal(msg.Data, &heartbeat); err != nil {
		t.Fatalf("decode heartbeat payload: %v", err)
	}

	if heartbeat.AssetID != "node-1" || heartbeat.Name != "node-1" {
		t.Fatalf("unexpected heartbeat identity: %+v", heartbeat)
	}
	if heartbeat.Source != "agent" || heartbeat.GroupID != "group-1" {
		t.Fatalf("unexpected heartbeat routing: %+v", heartbeat)
	}
	if heartbeat.Platform != "linux" || heartbeat.Metadata["platform"] != "linux" {
		t.Fatalf("expected normalized linux platform, got platform=%q metadata=%q", heartbeat.Platform, heartbeat.Metadata["platform"])
	}
	if len(heartbeat.Capabilities) != 2 || heartbeat.Capabilities[0] != "terminal" || heartbeat.Capabilities[1] != "files" {
		t.Fatalf("unexpected capabilities: %+v", heartbeat.Capabilities)
	}
	if heartbeat.Metadata["agent_messages_sent"] == "" || heartbeat.Metadata["agent_uptime_sec"] == "" {
		t.Fatalf("expected transport diagnostics in heartbeat metadata, got %+v", heartbeat.Metadata)
	}
	if got := len(fallback.snapshot()); got != 0 {
		t.Fatalf("expected no fallback publishes while connected, got %d", got)
	}
}
