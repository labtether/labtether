package persistence

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/telemetry"
	"strings"
	"time"
)

func (s *PostgresStore) Series(assetID string, start, end time.Time, step time.Duration) ([]telemetry.Series, error) {
	definitions := telemetry.CanonicalMetrics()

	// Build a slice of metric names for the ANY($2) bind parameter so we issue
	// a single query instead of one query per metric (was 6 round-trips).
	metricNames := make([]string, len(definitions))
	for i, d := range definitions {
		metricNames[i] = d.Metric
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT metric, collected_at, value
		 FROM metric_samples
		 WHERE asset_id = $1
		   AND metric = ANY($2::text[])
		   AND collected_at >= $3
		   AND collected_at <= $4
		 ORDER BY metric, collected_at ASC`,
		assetID,
		metricNames,
		start.UTC(),
		end.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Accumulate raw points per metric name.
	rawPoints := make(map[string][]telemetry.Point, len(definitions))
	for rows.Next() {
		var metric string
		var collectedAt time.Time
		var value float64
		if err := rows.Scan(&metric, &collectedAt, &value); err != nil {
			return nil, err
		}
		rawPoints[metric] = append(rawPoints[metric], telemetry.Point{
			TS:    collectedAt.Unix(),
			Value: value,
		})
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	// Build the output in canonical order, preserving unit metadata.
	out := make([]telemetry.Series, 0, len(definitions))
	for _, def := range definitions {
		points := telemetry.BucketAveragePoints(rawPoints[def.Metric], step)
		out = append(out, telemetry.Series{
			Metric:  def.Metric,
			Unit:    def.Unit,
			Points:  points,
			Current: telemetry.LastPointValue(points),
		})
	}

	return out, nil
}

func (s *PostgresStore) SeriesMetric(assetID, metric string, start, end time.Time, step time.Duration) (telemetry.Series, error) {
	metric = strings.TrimSpace(metric)
	definition, ok := telemetryCanonicalMetricDefinition(metric)
	if !ok {
		return telemetry.Series{Metric: metric}, nil
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT collected_at, value
		 FROM metric_samples
		 WHERE asset_id = $1
		   AND metric = $2
		   AND collected_at >= $3
		   AND collected_at <= $4
		 ORDER BY collected_at ASC`,
		assetID,
		metric,
		start.UTC(),
		end.UTC(),
	)
	if err != nil {
		return telemetry.Series{}, err
	}
	defer rows.Close()

	points := make([]telemetry.Point, 0, 64)
	for rows.Next() {
		var collectedAt time.Time
		var value float64
		if err := rows.Scan(&collectedAt, &value); err != nil {
			return telemetry.Series{}, err
		}
		points = append(points, telemetry.Point{
			TS:    collectedAt.Unix(),
			Value: value,
		})
	}
	if rows.Err() != nil {
		return telemetry.Series{}, rows.Err()
	}

	points = telemetry.BucketAveragePoints(points, step)
	return telemetry.Series{
		Metric:  metric,
		Unit:    definition.Unit,
		Points:  points,
		Current: telemetry.LastPointValue(points),
	}, nil
}

func (s *PostgresStore) HasTelemetrySamples(assetIDs []string, start, end time.Time) (map[string]bool, error) {
	cleanIDs := normalizeLogAssetIDs(assetIDs)
	out := make(map[string]bool, len(cleanIDs))
	if len(cleanIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(context.Background(),
		`SELECT DISTINCT asset_id
		 FROM metric_samples
		 WHERE asset_id = ANY($1::text[])
		   AND collected_at >= $2
		   AND collected_at <= $3`,
		cleanIDs,
		start.UTC(),
		end.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var assetID string
		if err := rows.Scan(&assetID); err != nil {
			return nil, err
		}
		out[strings.TrimSpace(assetID)] = true
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) MetricSeriesBatch(assetIDs []string, metric string, start, end time.Time, step time.Duration) (map[string]telemetry.Series, error) {
	return s.metricSeriesBatch(context.Background(), assetIDs, metric, start, end, step, 0)
}

// MetricSeriesBatchContext is the cancellation-aware, row-bounded query path
// for interactive/API requests. maxRawPoints must be positive; one extra row
// is requested so an oversized result fails rather than returning a misleading
// partial series.
func (s *PostgresStore) MetricSeriesBatchContext(ctx context.Context, assetIDs []string, metric string, start, end time.Time, step time.Duration, maxRawPoints int) (map[string]telemetry.Series, error) {
	if ctx == nil {
		return nil, errors.New("telemetry query context is required")
	}
	if maxRawPoints <= 0 {
		return nil, errors.New("telemetry query row limit must be positive")
	}
	return s.metricSeriesBatch(ctx, assetIDs, metric, start, end, step, maxRawPoints)
}

func (s *PostgresStore) metricSeriesBatch(ctx context.Context, assetIDs []string, metric string, start, end time.Time, step time.Duration, maxRawPoints int) (map[string]telemetry.Series, error) {
	cleanIDs := normalizeLogAssetIDs(assetIDs)
	out := make(map[string]telemetry.Series, len(cleanIDs))
	metric = strings.TrimSpace(metric)
	if len(cleanIDs) == 0 || metric == "" {
		return out, nil
	}

	definition, ok := telemetryCanonicalMetricDefinition(metric)
	if !ok {
		return out, nil
	}

	query := `SELECT asset_id, collected_at, value
		 FROM metric_samples
		 WHERE asset_id = ANY($1::text[])
		   AND metric = $2
		   AND collected_at >= $3
		   AND collected_at <= $4
		 ORDER BY asset_id ASC, collected_at ASC`
	args := []any{cleanIDs, metric, start.UTC(), end.UTC()}
	if maxRawPoints > 0 {
		query += " LIMIT $5"
		args = append(args, maxRawPoints+1)
	}
	rows, err := s.pool.Query(ctx,
		query,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rawPoints := make(map[string][]telemetry.Point, len(cleanIDs))
	rawPointCount := 0
	for rows.Next() {
		var assetID string
		var collectedAt time.Time
		var value float64
		if err := rows.Scan(&assetID, &collectedAt, &value); err != nil {
			return nil, err
		}
		assetID = strings.TrimSpace(assetID)
		if assetID == "" {
			continue
		}
		rawPointCount++
		if maxRawPoints > 0 && rawPointCount > maxRawPoints {
			return nil, ErrTelemetryQueryLimitExceeded
		}
		rawPoints[assetID] = append(rawPoints[assetID], telemetry.Point{
			TS:    collectedAt.Unix(),
			Value: value,
		})
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	for _, assetID := range cleanIDs {
		points := telemetry.BucketAveragePoints(rawPoints[assetID], step)
		out[assetID] = telemetry.Series{
			Metric:  metric,
			Unit:    definition.Unit,
			Points:  points,
			Current: telemetry.LastPointValue(points),
		}
	}

	return out, nil
}

func (s *PostgresStore) AssetsWithSamples(assetIDs []string, start, end time.Time) (map[string]bool, error) {
	return s.HasTelemetrySamples(assetIDs, start, end)
}

func (s *PostgresStore) TelemetryWatermark() (time.Time, error) {
	var watermark time.Time
	if err := s.pool.QueryRow(
		context.Background(),
		`SELECT GREATEST(
			COALESCE((SELECT MAX(collected_at) FROM metric_samples), to_timestamp(0)),
			COALESCE((SELECT MAX(collected_at) FROM hub_metric_samples), to_timestamp(0))
		)`,
	).Scan(&watermark); err != nil {
		return time.Time{}, err
	}
	return watermark.UTC(), nil
}
