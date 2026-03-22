package bridge

import (
	"context"
	"log/slog"
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bridges = append(r.bridges, b)
}

// CollectAll gathers samples from all registered bridges (for testing/one-shot use).
func (r *Registry) CollectAll() []telemetry.MetricSample {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []telemetry.MetricSample
	for _, b := range r.bridges {
		samples := b.Collect()
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
		go r.runBridge(ctx, b, appendFn)
	}
}

// defaultStartupDelay gives asset discovery a head start so most assets exist
// before the first bridge flush. Samples for assets still missing are handled
// gracefully by the persistence layer (individual retry, skip FK violations).
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

	ticker := time.NewTicker(b.Interval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			samples := b.Collect()
			if len(samples) == 0 {
				continue
			}
			if err := appendFn(ctx, samples); err != nil {
				slog.Warn("bridge flush failed", "bridge", b.Name(), "error", err)
			}
		}
	}
}
