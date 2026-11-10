package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/labtether/labtether/internal/telemetry"
	"strings"
	"sync"
	"time"
)

var (
	canonicalTelemetryMetricNamesOnce   sync.Once
	canonicalTelemetryMetricNamesCached []string
)

// HubMetricSnapshots returns the latest sample for each
// (scope, metric, labels) series. Hub scopes are intentionally separate from
// assets and are consumed by the Prometheus adapter without becoming
// user-visible device rows.
func (s *PostgresStore) HubMetricSnapshots(ctx context.Context, at time.Time, maxSeries int) (map[string][]telemetry.MetricSample, error) {
	if ctx == nil {
		return nil, fmt.Errorf("hub metric snapshot context is required")
	}
	if maxSeries <= 0 || maxSeries > telemetry.MaxHubMetricSnapshotSeries {
		return nil, ErrHubMetricSnapshotLimitExceeded
	}
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT ON (scope, metric, COALESCE(labels, '{}'::jsonb))
			scope, metric, unit, value, collected_at, labels
		 FROM hub_metric_samples
		 WHERE collected_at <= $1
		   AND collected_at >= $2
		 ORDER BY scope, metric, COALESCE(labels, '{}'::jsonb), collected_at DESC, id DESC
		 LIMIT $3`,
		at.UTC(),
		at.UTC().Add(-telemetry.HubMetricSnapshotMaxAge),
		maxSeries+1,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]telemetry.MetricSample, 2)
	seriesCount := 0
	for rows.Next() {
		var (
			sample    telemetry.MetricSample
			labelsRaw []byte
		)
		if err := rows.Scan(&sample.Scope, &sample.Metric, &sample.Unit, &sample.Value, &sample.CollectedAt, &labelsRaw); err != nil {
			return nil, err
		}
		if len(labelsRaw) > 0 {
			if err := json.Unmarshal(labelsRaw, &sample.Labels); err != nil {
				return nil, fmt.Errorf("decode labels for hub sample %q/%q: %w", sample.Scope, sample.Metric, err)
			}
		}
		normalized, err := telemetry.NormalizeHubMetricSample(sample)
		if err != nil {
			return nil, fmt.Errorf("invalid persisted hub sample %q/%q: %w", sample.Scope, sample.Metric, err)
		}
		if _, err := telemetry.MetricSampleEnvelopeBytes(normalized); err != nil {
			return nil, fmt.Errorf("invalid persisted hub sample envelope %q/%q: %w", sample.Scope, sample.Metric, err)
		}
		sample = normalized
		if seriesCount >= maxSeries {
			return nil, ErrHubMetricSnapshotLimitExceeded
		}
		out[sample.Scope] = append(out[sample.Scope], sample)
		seriesCount++
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) DynamicSnapshotForAsset(assetID string, at time.Time) (telemetry.DynamicSnapshot, error) {
	many, err := s.DynamicSnapshotMany([]string{assetID}, at)
	if err != nil {
		return telemetry.DynamicSnapshot{}, err
	}
	dyn := many[strings.TrimSpace(assetID)]
	if dyn.Metrics == nil {
		dyn.Metrics = make(map[string]float64)
	}
	return dyn, nil
}

func (s *PostgresStore) Snapshot(assetID string, at time.Time) (telemetry.Snapshot, error) {
	dyn, err := s.DynamicSnapshotForAsset(assetID, at)
	if err != nil {
		return telemetry.Snapshot{}, err
	}
	return dyn.ToLegacySnapshot(), nil
}

func (s *PostgresStore) DynamicSnapshotMany(assetIDs []string, at time.Time) (map[string]telemetry.DynamicSnapshot, error) {
	seen := make(map[string]struct{}, len(assetIDs))
	cleanIDs := make([]string, 0, len(assetIDs))
	for _, rawID := range assetIDs {
		assetID := strings.TrimSpace(rawID)
		if assetID == "" {
			continue
		}
		if _, exists := seen[assetID]; exists {
			continue
		}
		seen[assetID] = struct{}{}
		cleanIDs = append(cleanIDs, assetID)
	}

	out := make(map[string]telemetry.DynamicSnapshot, len(cleanIDs))
	if len(cleanIDs) == 0 {
		return out, nil
	}
	for _, assetID := range cleanIDs {
		out[assetID] = telemetry.DynamicSnapshot{Metrics: make(map[string]float64)}
	}

	// Latest value per (asset, metric) from the device's own samples. Then,
	// for metrics only a part of the device reports (disk totals per mount),
	// the newest collection's system disk, so a device's disk numbers fit
	// together. No metric name filter: returns whatever metrics exist.
	if err := s.scanSnapshotRows(out, `SELECT DISTINCT ON (asset_id, metric) asset_id, metric, value
		 FROM metric_samples
		 WHERE asset_id = ANY($1::text[])
		   AND collected_at <= $2
		   AND `+deviceLevelSampleFilter+`
		 ORDER BY asset_id, metric, collected_at DESC`, cleanIDs, at.UTC(), true); err != nil {
		return nil, err
	}
	if err := s.scanSnapshotRows(out, `SELECT DISTINCT ON (asset_id, metric) asset_id, metric, value
		 FROM metric_samples
		 WHERE asset_id = ANY($1::text[])
		   AND collected_at <= $2
		   AND collected_at > $2 - `+recentMountSampleWindow+`
		   AND NOT `+deviceLevelSampleFilter+`
		 ORDER BY asset_id, metric, collected_at DESC, `+primaryMountOrderSQL, cleanIDs, at.UTC(), false); err != nil {
		return nil, err
	}
	return out, nil
}

// scanSnapshotRows adds (asset_id, metric, value) rows to out. With
// overwrite false it only fills metrics an asset does not have yet.
func (s *PostgresStore) scanSnapshotRows(out map[string]telemetry.DynamicSnapshot, query string, assetIDs []string, at time.Time, overwrite bool) error {
	rows, err := s.pool.Query(context.Background(), query, assetIDs, at)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			assetID string
			metric  string
			value   float64
		)
		if err := rows.Scan(&assetID, &metric, &value); err != nil {
			return err
		}
		dyn, ok := out[assetID]
		if !ok {
			continue
		}
		if dyn.Metrics == nil {
			dyn.Metrics = make(map[string]float64)
		}
		if _, exists := dyn.Metrics[metric]; exists && !overwrite {
			continue
		}
		dyn.Metrics[metric] = value
		out[assetID] = dyn
	}
	return rows.Err()
}

func (s *PostgresStore) SnapshotMany(assetIDs []string, at time.Time) (map[string]telemetry.Snapshot, error) {
	dynMap, err := s.DynamicSnapshotMany(assetIDs, at)
	if err != nil {
		return nil, err
	}
	out := make(map[string]telemetry.Snapshot, len(dynMap))
	for assetID, dyn := range dynMap {
		out[assetID] = dyn.ToLegacySnapshot()
	}
	return out, nil
}

func telemetryCanonicalMetricNames() []string {
	canonicalTelemetryMetricNamesOnce.Do(func() {
		definitions := telemetry.CanonicalMetrics()
		names := make([]string, 0, len(definitions))
		for _, definition := range definitions {
			metric := strings.TrimSpace(definition.Metric)
			if metric == "" {
				continue
			}
			names = append(names, metric)
		}
		canonicalTelemetryMetricNamesCached = names
	})
	return canonicalTelemetryMetricNamesCached
}

func telemetryCanonicalMetricDefinition(metric string) (telemetry.MetricDefinition, bool) {
	metric = strings.TrimSpace(metric)
	for _, definition := range telemetry.CanonicalMetrics() {
		if definition.Metric == metric {
			return definition, true
		}
	}
	return telemetry.MetricDefinition{}, false
}
