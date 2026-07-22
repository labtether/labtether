package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

const maxSamplesPerAsset = 1000

type MemoryTelemetryStore struct {
	mu         sync.RWMutex
	samples    map[string][]telemetry.MetricSample
	hubSamples map[string][]telemetry.MetricSample
}

func NewMemoryTelemetryStore() *MemoryTelemetryStore {
	return &MemoryTelemetryStore{
		samples:    make(map[string][]telemetry.MetricSample),
		hubSamples: make(map[string][]telemetry.MetricSample),
	}
}

func (m *MemoryTelemetryStore) AppendSamples(ctx context.Context, samples []telemetry.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	if err := validateMetricSampleBatch(ctx, samples); err != nil {
		return err
	}

	prepared := make([]telemetry.MetricSample, 0, len(samples))
	incomingHubSeries := make(map[string]map[string]struct{}, 2)
	now := time.Now().UTC()
	for _, sample := range samples {
		sample.Scope = strings.TrimSpace(sample.Scope)
		if sample.Scope != "" {
			var err error
			sample, err = telemetry.NormalizeHubMetricSample(sample)
			if err != nil {
				return err
			}
			seriesKey, err := hubMetricSeriesKey(sample)
			if err != nil {
				return err
			}
			series := incomingHubSeries[sample.Scope]
			if series == nil {
				series = make(map[string]struct{})
				incomingHubSeries[sample.Scope] = series
			}
			series[seriesKey] = struct{}{}
			if len(series) > telemetry.MaxHubMetricSeriesPerScope {
				return ErrHubMetricSnapshotLimitExceeded
			}
		} else {
			sample.AssetID = strings.TrimSpace(sample.AssetID)
			sample.Metric = strings.TrimSpace(sample.Metric)
			sample.Unit = strings.TrimSpace(sample.Unit)
			if sample.AssetID == "" || sample.Metric == "" || sample.Unit == "" {
				return fmt.Errorf("asset metric sample requires non-empty asset_id, metric, and unit")
			}
		}
		if sample.CollectedAt.IsZero() {
			sample.CollectedAt = now
		} else {
			sample.CollectedAt = sample.CollectedAt.UTC()
		}
		sample.Labels = cloneMetadata(sample.Labels)
		prepared = append(prepared, sample)
	}
	if len(prepared) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	written := make(map[string]struct{}, len(prepared))
	writtenHubScopes := make(map[string]struct{}, len(prepared))
	for _, sample := range prepared {
		if sample.Scope != "" {
			m.hubSamples[sample.Scope] = append(m.hubSamples[sample.Scope], sample)
			writtenHubScopes[sample.Scope] = struct{}{}
		} else {
			m.samples[sample.AssetID] = append(m.samples[sample.AssetID], sample)
			written[sample.AssetID] = struct{}{}
		}
	}

	// Evict oldest entries from written assets that exceed the cap,
	// trimming back to 80% to avoid evicting on every append.
	for assetID := range written {
		slice := m.samples[assetID]
		if len(slice) > maxSamplesPerAsset {
			// Preserve event-time recency under delayed/out-of-order delivery.
			// SliceStable keeps later appends later when timestamps tie.
			sort.SliceStable(slice, func(i, j int) bool {
				return slice[i].CollectedAt.Before(slice[j].CollectedAt)
			})
			dropCount := len(slice) - maxSamplesPerAsset*4/5
			m.samples[assetID] = append(slice[:0:0], slice[dropCount:]...)
		}
	}
	for scope := range writtenHubScopes {
		m.hubSamples[scope] = compactHubMetricHistory(m.hubSamples[scope])
	}

	return nil
}

func hubMetricSeriesKey(sample telemetry.MetricSample) (string, error) {
	if len(sample.Labels) == 0 {
		return sample.Metric + "\x00{}", nil
	}
	labelsJSON, err := json.Marshal(sample.Labels)
	if err != nil {
		return "", fmt.Errorf("marshal hub metric labels: %w", err)
	}
	return sample.Metric + "\x00" + string(labelsJSON), nil
}

func compactHubMetricHistory(samples []telemetry.MetricSample) []telemetry.MetricSample {
	if len(samples) == 0 {
		return nil
	}
	type indexedSample struct {
		index  int
		sample telemetry.MetricSample
	}
	newest := make([]indexedSample, len(samples))
	for i, sample := range samples {
		newest[i] = indexedSample{index: i, sample: sample}
	}
	sort.SliceStable(newest, func(i, j int) bool {
		if newest[i].sample.CollectedAt.Equal(newest[j].sample.CollectedAt) {
			return newest[i].index > newest[j].index
		}
		return newest[i].sample.CollectedAt.After(newest[j].sample.CollectedAt)
	})
	seriesCounts := make(map[string]int, telemetry.MaxHubMetricSeriesPerScope)
	kept := make([]bool, len(samples))
	keptCount := 0
	for _, candidate := range newest {
		key, err := hubMetricSeriesKey(candidate.sample)
		if err != nil {
			continue
		}
		count, exists := seriesCounts[key]
		if !exists && len(seriesCounts) >= telemetry.MaxHubMetricSeriesPerScope {
			continue
		}
		if count >= telemetry.MaxHubMetricHistoryPerSeries {
			continue
		}
		seriesCounts[key] = count + 1
		kept[candidate.index] = true
		keptCount++
	}
	out := make([]telemetry.MetricSample, 0, keptCount)
	for i, sample := range samples {
		if kept[i] {
			out = append(out, sample)
		}
	}
	return out
}

func (m *MemoryTelemetryStore) HubMetricSnapshots(ctx context.Context, at time.Time, maxSeries int) (map[string][]telemetry.MetricSample, error) {
	if ctx == nil {
		return nil, fmt.Errorf("hub metric snapshot context is required")
	}
	if maxSeries <= 0 || maxSeries > telemetry.MaxHubMetricSnapshotSeries {
		return nil, ErrHubMetricSnapshotLimitExceeded
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string][]telemetry.MetricSample, len(m.hubSamples))
	oldest := at.Add(-telemetry.HubMetricSnapshotMaxAge)
	totalSeries := 0
	for scope, samples := range m.hubSamples {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		latest := make(map[string]telemetry.MetricSample)
		for i, sample := range samples {
			if i%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			if sample.CollectedAt.After(at) || sample.CollectedAt.Before(oldest) {
				continue
			}
			key, err := hubMetricSeriesKey(sample)
			if err != nil {
				return nil, err
			}
			current, ok := latest[key]
			if !ok || sample.CollectedAt.After(current.CollectedAt) || sample.CollectedAt.Equal(current.CollectedAt) {
				latest[key] = sample
			}
		}
		keys := make([]string, 0, len(latest))
		for key := range latest {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			sample := latest[key]
			sample.Labels = cloneMetadata(sample.Labels)
			out[scope] = append(out[scope], sample)
			totalSeries++
			if totalSeries > maxSeries {
				return nil, ErrHubMetricSnapshotLimitExceeded
			}
		}
	}
	return out, nil
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
