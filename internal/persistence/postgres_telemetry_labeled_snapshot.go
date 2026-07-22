package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// LatestLabeledMetricSnapshots returns the latest sample per
// (asset, raw metric, canonical labels). The candidate CTE stops after one row
// beyond the raw-work budget; any overflow fails closed instead of exporting a
// misleading partial scrape. DISTINCT work is therefore bounded as well.
func (s *PostgresStore) LatestLabeledMetricSnapshots(ctx context.Context, assetIDs []string, at time.Time, maxSeries int) (map[string][]telemetry.MetricSample, error) {
	return s.latestLabeledMetricSnapshots(ctx, assetIDs, at, maxSeries, telemetry.MaxPrometheusAssetMetricRows)
}

func (s *PostgresStore) latestLabeledMetricSnapshots(ctx context.Context, assetIDs []string, at time.Time, maxSeries, maxRows int) (map[string][]telemetry.MetricSample, error) {
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
	out := make(map[string][]telemetry.MetricSample, len(cleanIDs))
	for _, assetID := range cleanIDs {
		out[assetID] = nil
	}
	if len(cleanIDs) == 0 {
		return out, nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}

	rows, err := s.pool.Query(ctx, `
		WITH candidates AS MATERIALIZED (
			SELECT id, asset_id, metric, unit, value, collected_at, labels
			  FROM metric_samples
			 WHERE asset_id = ANY($1::text[])
			   AND collected_at <= $2
			   AND collected_at >= $3
			 LIMIT $4
		), latest AS MATERIALIZED (
			SELECT DISTINCT ON (asset_id, metric, COALESCE(labels, '{}'::jsonb))
			       id, asset_id, metric, unit, value, collected_at, labels
			  FROM candidates
			 ORDER BY asset_id, metric, COALESCE(labels, '{}'::jsonb), collected_at DESC, id DESC
		), bounded AS (
			SELECT asset_id, metric, unit, value, collected_at, labels
			  FROM latest
			 ORDER BY asset_id, metric, COALESCE(labels, '{}'::jsonb)
			 LIMIT $5
		)
		SELECT asset_id, metric, unit, value, collected_at, labels,
		       (SELECT COUNT(*) > $6 FROM candidates) AS raw_overflow
		  FROM bounded`,
		cleanIDs,
		at,
		at.Add(-telemetry.PrometheusAssetSnapshotMaxAge),
		maxRows+1,
		maxSeries+1,
		maxRows,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seriesCount := 0
	for rows.Next() {
		var (
			sample      telemetry.MetricSample
			labelsRaw   []byte
			rawOverflow bool
		)
		if err := rows.Scan(
			&sample.AssetID,
			&sample.Metric,
			&sample.Unit,
			&sample.Value,
			&sample.CollectedAt,
			&labelsRaw,
			&rawOverflow,
		); err != nil {
			return nil, err
		}
		if rawOverflow {
			return nil, ErrTelemetrySnapshotRowLimitExceeded
		}
		if seriesCount >= maxSeries {
			return nil, ErrTelemetrySnapshotSeriesLimitExceeded
		}
		if len(labelsRaw) > 0 {
			if err := json.Unmarshal(labelsRaw, &sample.Labels); err != nil {
				return nil, fmt.Errorf("decode labels for asset metric sample %q/%q: %w", sample.AssetID, sample.Metric, err)
			}
		}
		if _, err := telemetry.MetricSampleEnvelopeBytes(sample); err != nil {
			return nil, fmt.Errorf("invalid persisted asset metric sample %q/%q: %w", sample.AssetID, sample.Metric, err)
		}
		sample.CollectedAt = sample.CollectedAt.UTC()
		out[sample.AssetID] = append(out[sample.AssetID], sample)
		seriesCount++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
