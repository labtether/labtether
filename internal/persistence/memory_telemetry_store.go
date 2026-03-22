package persistence

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

const maxSamplesPerAsset = 1000

type MemoryTelemetryStore struct {
	mu      sync.RWMutex
	samples map[string][]telemetry.MetricSample
}

func NewMemoryTelemetryStore() *MemoryTelemetryStore {
	return &MemoryTelemetryStore{
		samples: make(map[string][]telemetry.MetricSample),
	}
}

func (m *MemoryTelemetryStore) AppendSamples(_ context.Context, samples []telemetry.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	written := make(map[string]struct{}, len(samples))
	for _, sample := range samples {
		if sample.AssetID == "" || sample.Metric == "" {
			continue
		}
		m.samples[sample.AssetID] = append(m.samples[sample.AssetID], sample)
		written[sample.AssetID] = struct{}{}
	}

	// Evict oldest entries from written assets that exceed the cap,
	// trimming back to 80% to avoid evicting on every append.
	for assetID := range written {
		slice := m.samples[assetID]
		if len(slice) > maxSamplesPerAsset {
			dropCount := len(slice) - maxSamplesPerAsset*4/5
			m.samples[assetID] = append(slice[:0:0], slice[dropCount:]...)
		}
	}

	return nil
}

func (m *MemoryTelemetryStore) DynamicSnapshotForAsset(assetID string, at time.Time) (telemetry.DynamicSnapshot, error) {
	m.mu.RLock()
	samples := append([]telemetry.MetricSample(nil), m.samples[assetID]...)
	m.mu.RUnlock()

	return dynamicSnapshotFromSamples(samples, at), nil
}

func (m *MemoryTelemetryStore) DynamicSnapshotMany(assetIDs []string, at time.Time) (map[string]telemetry.DynamicSnapshot, error) {
	seen := make(map[string]struct{}, len(assetIDs))
	out := make(map[string]telemetry.DynamicSnapshot, len(assetIDs))

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, assetID := range assetIDs {
		if assetID == "" {
			continue
		}
		if _, exists := seen[assetID]; exists {
			continue
		}
		seen[assetID] = struct{}{}
		out[assetID] = dynamicSnapshotFromSamples(m.samples[assetID], at)
	}
	return out, nil
}

func (m *MemoryTelemetryStore) Snapshot(assetID string, at time.Time) (telemetry.Snapshot, error) {
	dyn, err := m.DynamicSnapshotForAsset(assetID, at)
	if err != nil {
		return telemetry.Snapshot{}, err
	}
	return dyn.ToLegacySnapshot(), nil
}

func (m *MemoryTelemetryStore) SnapshotMany(assetIDs []string, at time.Time) (map[string]telemetry.Snapshot, error) {
	dynMap, err := m.DynamicSnapshotMany(assetIDs, at)
	if err != nil {
		return nil, err
	}
	out := make(map[string]telemetry.Snapshot, len(dynMap))
	for assetID, dyn := range dynMap {
		out[assetID] = dyn.ToLegacySnapshot()
	}
	return out, nil
}

func dynamicSnapshotFromSamples(samples []telemetry.MetricSample, at time.Time) telemetry.DynamicSnapshot {
	latest := make(map[string]telemetry.MetricSample, 8)
	for _, sample := range samples {
		if sample.CollectedAt.After(at) {
			continue
		}
		current, ok := latest[sample.Metric]
		if !ok || sample.CollectedAt.After(current.CollectedAt) {
			latest[sample.Metric] = sample
		}
	}

	metrics := make(map[string]float64, len(latest))
	for metric, sample := range latest {
		metrics[metric] = sample.Value
	}
	return telemetry.DynamicSnapshot{Metrics: metrics}
}

func (m *MemoryTelemetryStore) Series(assetID string, start, end time.Time, step time.Duration) ([]telemetry.Series, error) {
	m.mu.RLock()
	samples := append([]telemetry.MetricSample(nil), m.samples[assetID]...)
	m.mu.RUnlock()

	out := make([]telemetry.Series, 0, len(telemetry.CanonicalMetrics()))
	for _, definition := range telemetry.CanonicalMetrics() {
		points := make([]telemetry.Point, 0, 64)
		for _, sample := range samples {
			if sample.Metric != definition.Metric {
				continue
			}
			if sample.CollectedAt.Before(start) || sample.CollectedAt.After(end) {
				continue
			}
			points = append(points, telemetry.Point{
				TS:    sample.CollectedAt.Unix(),
				Value: sample.Value,
			})
		}

		sort.Slice(points, func(i, j int) bool {
			return points[i].TS < points[j].TS
		})
		points = telemetry.BucketAveragePoints(points, step)

		out = append(out, telemetry.Series{
			Metric:  definition.Metric,
			Unit:    definition.Unit,
			Points:  points,
			Current: telemetry.LastPointValue(points),
		})
	}

	return out, nil
}

func (m *MemoryTelemetryStore) SeriesMetric(assetID, metric string, start, end time.Time, step time.Duration) (telemetry.Series, error) {
	m.mu.RLock()
	samples := append([]telemetry.MetricSample(nil), m.samples[assetID]...)
	m.mu.RUnlock()

	points := make([]telemetry.Point, 0, 64)
	for _, sample := range samples {
		if sample.Metric != metric {
			continue
		}
		if sample.CollectedAt.Before(start) || sample.CollectedAt.After(end) {
			continue
		}
		points = append(points, telemetry.Point{
			TS:    sample.CollectedAt.Unix(),
			Value: sample.Value,
		})
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].TS < points[j].TS
	})
	points = telemetry.BucketAveragePoints(points, step)
	definition, _ := telemetryCanonicalMetricDefinition(metric)
	return telemetry.Series{
		Metric:  metric,
		Unit:    definition.Unit,
		Points:  points,
		Current: telemetry.LastPointValue(points),
	}, nil
}

func (m *MemoryTelemetryStore) MetricSeriesBatch(assetIDs []string, metric string, start, end time.Time, step time.Duration) (map[string]telemetry.Series, error) {
	// Deduplicate IDs.
	seen := make(map[string]struct{}, len(assetIDs))
	clean := make([]string, 0, len(assetIDs))
	for _, rawID := range assetIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}

	// Hold a single RLock for the entire batch to get a consistent view and
	// avoid N separate lock acquisitions (one per SeriesMetric call).
	m.mu.RLock()
	snapshots := make(map[string][]telemetry.MetricSample, len(clean))
	for _, id := range clean {
		snapshots[id] = append([]telemetry.MetricSample(nil), m.samples[id]...)
	}
	m.mu.RUnlock()

	definition, _ := telemetryCanonicalMetricDefinition(metric)
	out := make(map[string]telemetry.Series, len(clean))
	for _, id := range clean {
		samples := snapshots[id]
		points := make([]telemetry.Point, 0, 64)
		for _, sample := range samples {
			if sample.Metric != metric {
				continue
			}
			if sample.CollectedAt.Before(start) || sample.CollectedAt.After(end) {
				continue
			}
			points = append(points, telemetry.Point{
				TS:    sample.CollectedAt.Unix(),
				Value: sample.Value,
			})
		}
		sort.Slice(points, func(i, j int) bool {
			return points[i].TS < points[j].TS
		})
		points = telemetry.BucketAveragePoints(points, step)
		out[id] = telemetry.Series{
			Metric:  metric,
			Unit:    definition.Unit,
			Points:  points,
			Current: telemetry.LastPointValue(points),
		}
	}
	return out, nil
}

func (m *MemoryTelemetryStore) HasTelemetrySamples(assetIDs []string, start, end time.Time) (map[string]bool, error) {
	out := make(map[string]bool, len(assetIDs))
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, assetID := range assetIDs {
		assetID = strings.TrimSpace(assetID)
		if assetID == "" {
			continue
		}
		for _, sample := range m.samples[assetID] {
			if sample.CollectedAt.Before(start) || sample.CollectedAt.After(end) {
				continue
			}
			out[assetID] = true
			break
		}
	}
	return out, nil
}

func (m *MemoryTelemetryStore) AssetsWithSamples(assetIDs []string, start, end time.Time) (map[string]bool, error) {
	return m.HasTelemetrySamples(assetIDs, start, end)
}
