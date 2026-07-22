package bridge

import (
	"context"
	"math"
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
			{AssetID: "asset-run", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 10.0, CollectedAt: time.Now().UTC()},
		},
		interval: time.Hour,
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

type panicBridge struct{}

func (*panicBridge) Name() string                      { return "panic-bridge" }
func (*panicBridge) Interval() time.Duration           { return time.Second }
func (*panicBridge) Collect() []telemetry.MetricSample { panic("forced") }

func TestRegistryFiltersInvalidSamplesAndRecoversSourcePanics(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockBridge{name: "mixed", interval: time.Second, samples: []telemetry.MetricSample{
		{AssetID: "asset-1", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 42, CollectedAt: time.Now().UTC()},
		{AssetID: "asset-1", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 42},
		{AssetID: "asset-1", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: math.NaN(), CollectedAt: time.Now().UTC()},
	}})
	r.Register(&panicBridge{})

	all := r.CollectAll()
	if len(all) != 3 {
		t.Fatalf("CollectAll should retain raw one-shot samples for diagnostics, got %d", len(all))
	}
	if got := validBridgeSamples(all); len(got) != 1 || got[0].Value != 42 {
		t.Fatalf("validated samples = %+v, want one finite timestamped sample", got)
	}
}

func TestValidBridgeSamplesDoesNotLetInvalidPrefixStarveValidTail(t *testing.T) {
	samples := make([]telemetry.MetricSample, telemetry.MaxMetricSamplesPerAppend+1)
	samples[len(samples)-1] = telemetry.MetricSample{
		AssetID: "asset-tail", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 42,
		CollectedAt: time.Now().UTC(),
	}
	got := validBridgeSamples(samples)
	if len(got) != 1 || got[0].AssetID != "asset-tail" {
		t.Fatalf("validated tail sample = %+v", got)
	}
}
