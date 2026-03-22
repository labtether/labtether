package bridge

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

type mockBridge struct {
	name     string
	samples  []telemetry.MetricSample
	interval time.Duration
}

func (m *mockBridge) Name() string                      { return m.name }
func (m *mockBridge) Collect() []telemetry.MetricSample { return m.samples }
func (m *mockBridge) Interval() time.Duration           { return m.interval }

func TestRegistryCollectsFromAllBridges(t *testing.T) {
	r := NewRegistry()

	b1 := &mockBridge{
		name: "bridge1",
		samples: []telemetry.MetricSample{
			{AssetID: "asset-1", Metric: telemetry.MetricCPUUsedPercent, Value: 42.0},
			{AssetID: "asset-1", Metric: telemetry.MetricMemoryUsedPercent, Value: 55.0},
		},
		interval: time.Second,
	}
	b2 := &mockBridge{
		name: "bridge2",
		samples: []telemetry.MetricSample{
			{AssetID: "asset-2", Metric: telemetry.MetricDiskUsedPercent, Value: 70.0},
		},
		interval: time.Second,
	}

	r.Register(b1)
	r.Register(b2)

	all := r.CollectAll()

	if len(all) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(all))
	}

	// Verify samples from both bridges are present.
	found := make(map[string]bool)
	for _, s := range all {
		found[s.AssetID+":"+s.Metric] = true
	}
	if !found["asset-1:"+telemetry.MetricCPUUsedPercent] {
		t.Error("missing asset-1 cpu sample")
	}
	if !found["asset-1:"+telemetry.MetricMemoryUsedPercent] {
		t.Error("missing asset-1 memory sample")
	}
	if !found["asset-2:"+telemetry.MetricDiskUsedPercent] {
		t.Error("missing asset-2 disk sample")
	}
}

func TestRegistryCollectsEmptyBridge(t *testing.T) {
	r := NewRegistry()

	empty := &mockBridge{
		name:     "empty-bridge",
		samples:  nil,
		interval: time.Second,
	}
	r.Register(empty)

	all := r.CollectAll()
	if len(all) != 0 {
		t.Fatalf("expected 0 samples from empty bridge, got %d", len(all))
	}
}

func TestRegistryRunFlushes(t *testing.T) {
	r := NewRegistry()
	r.StartupDelay = 0 // disable grace period for test

	b := &mockBridge{
		name: "fast-bridge",
		samples: []telemetry.MetricSample{
			{AssetID: "asset-run", Metric: telemetry.MetricCPUUsedPercent, Value: 10.0},
		},
		interval: 10 * time.Millisecond,
	}
	r.Register(b)

	var mu sync.Mutex
	var flushed []telemetry.MetricSample

	appendFn := func(ctx context.Context, samples []telemetry.MetricSample) error {
		mu.Lock()
		defer mu.Unlock()
		flushed = append(flushed, samples...)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	r.Run(ctx, appendFn)

	// Wait for context to expire.
	<-ctx.Done()
	// Give the goroutine a moment to finish its final tick processing.
	time.Sleep(5 * time.Millisecond)

	mu.Lock()
	n := len(flushed)
	mu.Unlock()

	if n == 0 {
		t.Fatal("expected at least one flush, got zero")
	}

	// Verify the flushed samples have the expected content.
	mu.Lock()
	first := flushed[0]
	mu.Unlock()

	if first.AssetID != "asset-run" {
		t.Errorf("unexpected AssetID: %q", first.AssetID)
	}
	if first.Metric != telemetry.MetricCPUUsedPercent {
		t.Errorf("unexpected Metric: %q", first.Metric)
	}
}
