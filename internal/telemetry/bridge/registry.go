package bridge

import (
	"context"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// Registry manages bridge adapters and coordinates collection.
type Registry struct {
	mu           sync.RWMutex
	bridges      []MetricsBridge
	StartupDelay time.Duration // delay before first collection tick; 0 disables
}

func NewRegistry() *Registry {
	return &Registry{
		StartupDelay: defaultStartupDelay,
	}
}

func (r *Registry) Register(b MetricsBridge) {
	if b == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bridges = append(r.bridges, b)
}

// Names returns a deterministic registration snapshot for startup proof and
// diagnostics. Collection order remains registration order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.bridges))
	for _, b := range r.bridges {
		if b != nil {
			out = append(out, b.Name())
		}
	}
	return out
}

// CollectAll gathers samples from all registered bridges (for testing/one-shot use).
func (r *Registry) CollectAll() []telemetry.MetricSample {
	r.mu.RLock()
	bridges := append([]MetricsBridge(nil), r.bridges...)
	r.mu.RUnlock()
	var all []telemetry.MetricSample
	for _, b := range bridges {
		samples := safeCollect(b)
		all = append(all, samples...)
	}
	return all
}

// AppendFunc writes samples to the telemetry store.
type AppendFunc func(ctx context.Context, samples []telemetry.MetricSample) error

// Run starts background goroutines that collect from each bridge on its interval
// and flush to the telemetry store via appendFn.
func (r *Registry) Run(ctx context.Context, appendFn AppendFunc) {
	r.mu.RLock()
	bridges := make([]MetricsBridge, len(r.bridges))
	copy(bridges, r.bridges)
	r.mu.RUnlock()

	for _, b := range bridges {
		if b == nil || b.Interval() <= 0 {
			continue
		}
		go r.runBridge(ctx, b, appendFn)
	}
}

// defaultStartupDelay gives asset discovery a head start so most assets exist
// before the first bridge flush. The persistence layer writes samples for
// currently registered assets without triggering FK violations and returns a
// typed partial-write error naming any stale or still-missing asset IDs.
const defaultStartupDelay = 10 * time.Second

func (r *Registry) runBridge(ctx context.Context, b MetricsBridge, appendFn AppendFunc) {
	// Wait for asset discovery to populate the assets table before first flush.
	if r.StartupDelay > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.StartupDelay):
		}
	}

	collectAndFlush := func() {
		samples := validBridgeSamples(safeCollect(b))
		if len(samples) == 0 {
			return
		}
		if err := appendFn(ctx, samples); err != nil {
			slog.Warn("bridge flush failed", "bridge", b.Name(), "error", err)
		}
	}

	// Collect once after the discovery grace period; waiting one full interval
	// here needlessly delayed the first truthful export by up to a minute.
	collectAndFlush()
	ticker := time.NewTicker(b.Interval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectAndFlush()
		}
	}
}

func safeCollect(b MetricsBridge) (samples []telemetry.MetricSample) {
	if b == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("bridge collection panic recovered", "bridge", b.Name(), "panic", recovered, "stack", string(debug.Stack()))
			samples = nil
		}
	}()
	return b.Collect()
}

func validBridgeSamples(samples []telemetry.MetricSample) []telemetry.MetricSample {
	capacity := min(len(samples), telemetry.MaxMetricSamplesPerAppend)
	out := make([]telemetry.MetricSample, 0, capacity)
	for _, sample := range samples {
		if len(out) >= telemetry.MaxMetricSamplesPerAppend {
			break
		}
		if sample.CollectedAt.IsZero() || strings.TrimSpace(sample.Metric) == "" || strings.TrimSpace(sample.Unit) == "" {
			continue
		}
		if strings.TrimSpace(sample.Scope) != "" {
			normalized, err := telemetry.NormalizeHubMetricSample(sample)
			if err != nil {
				continue
			}
			sample = normalized
		} else if strings.TrimSpace(sample.AssetID) == "" {
			continue
		}
		if _, err := telemetry.MetricSampleEnvelopeBytes(sample); err != nil {
			continue
		}
		out = append(out, sample)
	}
	return out
}
