package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TelemetryMetricActivityStore answers exact metric presence without loading
// raw series or requiring the metric to belong to the canonical metric catalog.
type TelemetryMetricActivityStore interface {
	AssetsWithMetricSamples(assetIDs []string, metric string, start, end time.Time) (map[string]bool, error)
}

var (
	_ TelemetryMetricActivityStore = (*PostgresStore)(nil)
	_ TelemetryMetricActivityStore = (*MemoryTelemetryStore)(nil)
)

func (s *PostgresStore) AssetsWithMetricSamples(assetIDs []string, metric string, start, end time.Time) (map[string]bool, error) {
	return s.AssetsWithMetricSamplesContext(context.Background(), assetIDs, metric, start, end)
}

func (s *PostgresStore) AssetsWithMetricSamplesContext(ctx context.Context, assetIDs []string, metric string, start, end time.Time) (map[string]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	metric = strings.TrimSpace(metric)
	if metric == "" || end.Before(start) {
		return nil, fmt.Errorf("invalid metric activity query")
	}
	ids := normalizeLogAssetIDs(assetIDs)
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT requested.asset_id FROM unnest($1::text[]) AS requested(asset_id)
	 WHERE EXISTS (SELECT 1 FROM metric_samples
	 WHERE asset_id = requested.asset_id AND metric = $2
	 AND collected_at >= $3 AND collected_at <= $4)`, ids, metric, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (m *MemoryTelemetryStore) AssetsWithMetricSamples(assetIDs []string, metric string, start, end time.Time) (map[string]bool, error) {
	return m.AssetsWithMetricSamplesContext(context.Background(), assetIDs, metric, start, end)
}

func (m *MemoryTelemetryStore) AssetsWithMetricSamplesContext(ctx context.Context, assetIDs []string, metric string, start, end time.Time) (map[string]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	metric = strings.TrimSpace(metric)
	if metric == "" || end.Before(start) {
		return nil, fmt.Errorf("invalid metric activity query")
	}
	ids := normalizeLogAssetIDs(assetIDs)
	out := make(map[string]bool, len(ids))
	if err := lockAlertEvaluation(ctx, &m.mu, true); err != nil {
		return nil, err
	}
	defer m.mu.RUnlock()
	for _, id := range ids {
		for _, sample := range m.samples[id] {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if sample.Metric == metric && !sample.CollectedAt.Before(start) && !sample.CollectedAt.After(end) {
				out[id] = true
				break
			}
		}
	}
	return out, nil
}
