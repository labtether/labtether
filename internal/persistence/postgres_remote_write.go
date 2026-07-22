package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/labtether/labtether/internal/telemetry/remotewrite"
)

var remoteWriteFingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// LoadRemoteWriteCursor atomically initializes or loads the durable cursor for
// an endpoint. A changed endpoint fingerprint starts a fresh replay, ensuring
// a replacement receiver is never falsely treated as having accepted the old
// receiver's history.
func (s *PostgresStore) LoadRemoteWriteCursor(ctx context.Context, endpointFingerprint string) (remotewrite.Cursor, error) {
	if s == nil || s.pool == nil || ctx == nil {
		return remotewrite.Cursor{}, fmt.Errorf("remote write persistence is unavailable")
	}
	if !remoteWriteFingerprintPattern.MatchString(endpointFingerprint) {
		return remotewrite.Cursor{}, fmt.Errorf("remote write endpoint fingerprint is invalid")
	}
	var cursor remotewrite.Cursor
	err := s.pool.QueryRow(ctx, `
		INSERT INTO prometheus_remote_write_state (
			singleton, endpoint_fingerprint, asset_sample_id, hub_sample_id, updated_at
		) VALUES (TRUE, $1, 0, 0, NOW())
		ON CONFLICT (singleton) DO UPDATE SET
			endpoint_fingerprint = EXCLUDED.endpoint_fingerprint,
			asset_sample_id = CASE
				WHEN prometheus_remote_write_state.endpoint_fingerprint = EXCLUDED.endpoint_fingerprint
				THEN prometheus_remote_write_state.asset_sample_id ELSE 0 END,
			hub_sample_id = CASE
				WHEN prometheus_remote_write_state.endpoint_fingerprint = EXCLUDED.endpoint_fingerprint
				THEN prometheus_remote_write_state.hub_sample_id ELSE 0 END,
			last_advanced_at = CASE
				WHEN prometheus_remote_write_state.endpoint_fingerprint = EXCLUDED.endpoint_fingerprint
				THEN prometheus_remote_write_state.last_advanced_at ELSE NULL END,
			updated_at = CASE
				WHEN prometheus_remote_write_state.endpoint_fingerprint = EXCLUDED.endpoint_fingerprint
				THEN prometheus_remote_write_state.updated_at ELSE NOW() END
		RETURNING asset_sample_id, hub_sample_id`, endpointFingerprint).Scan(&cursor.AssetSampleID, &cursor.HubSampleID)
	if err != nil {
		return remotewrite.Cursor{}, fmt.Errorf("load remote write cursor: %w", err)
	}
	return cursor, nil
}

// SaveRemoteWriteCursor advances both insertion-ordered cursors monotonically.
// The endpoint predicate prevents a canceled worker from advancing state after
// a live configuration switch.
func (s *PostgresStore) SaveRemoteWriteCursor(ctx context.Context, endpointFingerprint string, cursor remotewrite.Cursor, advancedAt time.Time) error {
	if s == nil || s.pool == nil || ctx == nil {
		return fmt.Errorf("remote write persistence is unavailable")
	}
	if !remoteWriteFingerprintPattern.MatchString(endpointFingerprint) || cursor.AssetSampleID < 0 || cursor.HubSampleID < 0 || advancedAt.IsZero() {
		return fmt.Errorf("remote write cursor update is invalid")
	}
	result, err := s.pool.Exec(ctx, `
		UPDATE prometheus_remote_write_state
		   SET asset_sample_id = $2,
		       hub_sample_id = $3,
		       last_advanced_at = $4,
		       updated_at = NOW()
		 WHERE singleton = TRUE
		   AND endpoint_fingerprint = $1
		   AND asset_sample_id <= $2
		   AND hub_sample_id <= $3`, endpointFingerprint, cursor.AssetSampleID, cursor.HubSampleID, advancedAt.UTC())
	if err != nil {
		return fmt.Errorf("save remote write cursor: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("remote write cursor was not advanced")
	}
	return nil
}

// SamplesAfter loads a fair, bounded page from both insertion-ordered
// telemetry streams. It intentionally pages by BIGSERIAL IDs, never event
// timestamps, so equal or delayed timestamps cannot be skipped.
func (s *PostgresStore) SamplesAfter(ctx context.Context, cursor remotewrite.Cursor, limit int) (remotewrite.Batch, error) {
	if s == nil || s.pool == nil || ctx == nil {
		return remotewrite.Batch{}, fmt.Errorf("remote write persistence is unavailable")
	}
	if cursor.AssetSampleID < 0 || cursor.HubSampleID < 0 || limit <= 0 || limit > remotewrite.MaxSamplesPerRequest {
		return remotewrite.Batch{}, fmt.Errorf("remote write replay request is invalid")
	}
	assetLimit := (limit + 1) / 2
	hubLimit := limit / 2
	assetRows, err := s.remoteWriteAssetRows(ctx, cursor.AssetSampleID, assetLimit)
	if err != nil {
		return remotewrite.Batch{}, err
	}
	hubRows, err := s.remoteWriteHubRows(ctx, cursor.HubSampleID, hubLimit)
	if err != nil {
		return remotewrite.Batch{}, err
	}
	if len(assetRows) == assetLimit && len(hubRows) < hubLimit {
		extraLimit := hubLimit - len(hubRows)
		afterID := cursor.AssetSampleID
		if len(assetRows) > 0 {
			afterID = assetRows[len(assetRows)-1].ID
		}
		extra, extraErr := s.remoteWriteAssetRows(ctx, afterID, extraLimit)
		if extraErr != nil {
			return remotewrite.Batch{}, extraErr
		}
		assetRows = append(assetRows, extra...)
	} else if len(hubRows) == hubLimit && len(assetRows) < assetLimit {
		extraLimit := assetLimit - len(assetRows)
		afterID := cursor.HubSampleID
		if len(hubRows) > 0 {
			afterID = hubRows[len(hubRows)-1].ID
		}
		extra, extraErr := s.remoteWriteHubRows(ctx, afterID, extraLimit)
		if extraErr != nil {
			return remotewrite.Batch{}, extraErr
		}
		hubRows = append(hubRows, extra...)
	}
	batch, err := remotewrite.BuildBatch(cursor, assetRows, hubRows)
	if err != nil {
		return remotewrite.Batch{}, err
	}
	batch.More = len(assetRows)+len(hubRows) == limit
	return batch, nil
}

func (s *PostgresStore) remoteWriteAssetRows(ctx context.Context, afterID int64, limit int) ([]remotewrite.AssetSampleRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ms.id, ms.asset_id, a.name, a.type, COALESCE(a.platform, ''),
		       COALESCE(a.metadata->>'agent_id', ''), COALESCE(a.metadata->>'image', ''),
		       COALESCE(a.metadata->>'stack', ''), ms.metric, ms.unit, ms.value,
		       ms.collected_at, COALESCE(ms.labels, '{}'::jsonb)
		  FROM metric_samples ms
		  JOIN assets a ON a.id = ms.asset_id
		 WHERE ms.id > $1
		 ORDER BY ms.id ASC
		 LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("load remote write asset page: %w", err)
	}
	defer rows.Close()
	result := make([]remotewrite.AssetSampleRow, 0, limit)
	for rows.Next() {
		var row remotewrite.AssetSampleRow
		var labelsJSON []byte
		if err := rows.Scan(
			&row.ID, &row.AssetID, &row.AssetName, &row.AssetType, &row.Platform,
			&row.DockerHost, &row.DockerImage, &row.DockerStack, &row.Metric,
			&row.Unit, &row.Value, &row.CollectedAt, &labelsJSON,
		); err != nil {
			return nil, fmt.Errorf("scan remote write asset page: %w", err)
		}
		if err := json.Unmarshal(labelsJSON, &row.Labels); err != nil {
			return nil, fmt.Errorf("decode remote write asset labels: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate remote write asset page: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) remoteWriteHubRows(ctx context.Context, afterID int64, limit int) ([]remotewrite.HubSampleRow, error) {
	if limit == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, scope, metric, unit, value, collected_at,
		       COALESCE(labels, '{}'::jsonb)
		  FROM hub_metric_samples
		 WHERE id > $1
		 ORDER BY id ASC
		 LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("load remote write hub page: %w", err)
	}
	defer rows.Close()
	result := make([]remotewrite.HubSampleRow, 0, limit)
	for rows.Next() {
		var row remotewrite.HubSampleRow
		var labelsJSON []byte
		if err := rows.Scan(&row.ID, &row.Scope, &row.Metric, &row.Unit, &row.Value, &row.CollectedAt, &labelsJSON); err != nil {
			return nil, fmt.Errorf("scan remote write hub page: %w", err)
		}
		if err := json.Unmarshal(labelsJSON, &row.Labels); err != nil {
			return nil, fmt.Errorf("decode remote write hub labels: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate remote write hub page: %w", err)
	}
	return result, nil
}
