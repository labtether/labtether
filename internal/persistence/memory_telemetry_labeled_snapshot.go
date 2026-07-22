package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

func assetMetricSeriesKey(sample telemetry.MetricSample) (string, error) {
	if len(sample.Labels) == 0 {
		return sample.Metric + "\x00{}", nil
	}
	labelsJSON, err := json.Marshal(sample.Labels)
	if err != nil {
		return "", fmt.Errorf("marshal asset metric labels: %w", err)
	}
	return sample.Metric + "\x00" + string(labelsJSON), nil
}

// LatestLabeledMetricSnapshots returns the latest sample for every distinct
// (asset, raw metric, canonical labels) series within the bounded Prometheus
// freshness window. Equal timestamps use last append as the deterministic
// in-memory equivalent of Postgres's highest sample ID.
func (m *MemoryTelemetryStore) LatestLabeledMetricSnapshots(ctx context.Context, assetIDs []string, at time.Time, maxSeries int) (map[string][]telemetry.MetricSample, error) {
	return m.latestLabeledMetricSnapshots(ctx, assetIDs, at, maxSeries, telemetry.MaxPrometheusAssetMetricRows)
}

func (m *MemoryTelemetryStore) latestLabeledMetricSnapshots(ctx context.Context, assetIDs []string, at time.Time, maxSeries, maxRows int) (map[string][]telemetry.MetricSample, error) {
	if ctx == nil {
		return nil, fmt.Errorf("telemetry snapshot context is required")
	}
	if maxSeries <= 0 || maxSeries > telemetry.MaxPrometheusAssetMetricSeries {
		return nil, ErrTelemetrySnapshotSeriesLimitExceeded
	}
	if maxRows <= 0 || maxRows > telemetry.MaxPrometheusAssetMetricRows {
		return nil, ErrTelemetrySnapshotRowLimitExceeded
	}
	if len(assetIDs) > telemetry.MaxPrometheusSnapshotAssets {
		return nil, ErrTelemetrySnapshotAssetLimitExceeded
	}
	cleanIDs := normalizeLogAssetIDs(assetIDs)
	if len(cleanIDs) > telemetry.MaxPrometheusSnapshotAssets {
		return nil, ErrTelemetrySnapshotAssetLimitExceeded
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	oldest := at.Add(-telemetry.PrometheusAssetSnapshotMaxAge)
	out := make(map[string][]telemetry.MetricSample, len(cleanIDs))
	if len(cleanIDs) == 0 {
		return out, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	totalRows := 0
	totalSeries := 0
	for _, assetID := range cleanIDs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		latest := make(map[string]telemetry.MetricSample)
		for i, sample := range m.samples[assetID] {
			if i%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			if sample.CollectedAt.After(at) || sample.CollectedAt.Before(oldest) {
				continue
			}
			totalRows++
			if totalRows > maxRows {
				return nil, ErrTelemetrySnapshotRowLimitExceeded
			}
			key, err := assetMetricSeriesKey(sample)
			if err != nil {
				return nil, err
			}
			current, exists := latest[key]
			if !exists {
				totalSeries++
				if totalSeries > maxSeries {
					return nil, ErrTelemetrySnapshotSeriesLimitExceeded
				}
			}
			if !exists || sample.CollectedAt.After(current.CollectedAt) || sample.CollectedAt.Equal(current.CollectedAt) {
				sample.AssetID = assetID
				sample.Scope = ""
				sample.Labels = cloneMetadata(sample.Labels)
				latest[key] = sample
			}
		}
		keys := make([]string, 0, len(latest))
		for key := range latest {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out[assetID] = make([]telemetry.MetricSample, 0, len(keys))
		for _, key := range keys {
			sample := latest[key]
			sample.Labels = cloneMetadata(sample.Labels)
			out[assetID] = append(out[assetID], sample)
		}
	}
	return out, nil
}
