package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/telemetry"
	"sort"
	"strings"
	"time"
)

const maxMetricSamplesPerInsert = 10000 // 60,000 bind params; PostgreSQL maximum is 65,535.

func (s *PostgresStore) AppendSamples(ctx context.Context, samples []telemetry.MetricSample) error {
	if ctx == nil {
		return fmt.Errorf("metric append context is required")
	}
	if len(samples) == 0 {
		return nil
	}
	if err := validateMetricSampleBatch(ctx, samples); err != nil {
		return err
	}

	assetSamples := make([]telemetry.MetricSample, 0, len(samples))
	hubSamples := make([]telemetry.MetricSample, 0, len(samples))
	incomingHubSeries := make(map[string]map[string]struct{}, 2)
	now := time.Now().UTC()
	for _, sample := range samples {
		sm := sample
		sm.Scope = strings.TrimSpace(sm.Scope)
		if sm.Scope != "" {
			var err error
			sm, err = telemetry.NormalizeHubMetricSample(sm)
			if err != nil {
				return err
			}
			seriesKey, err := hubMetricSeriesKey(sm)
			if err != nil {
				return err
			}
			series := incomingHubSeries[sm.Scope]
			if series == nil {
				series = make(map[string]struct{})
				incomingHubSeries[sm.Scope] = series
			}
			series[seriesKey] = struct{}{}
			if len(series) > telemetry.MaxHubMetricSeriesPerScope {
				return ErrHubMetricSnapshotLimitExceeded
			}
		} else {
			sm.AssetID = strings.TrimSpace(sm.AssetID)
			sm.Metric = strings.TrimSpace(sm.Metric)
			sm.Unit = strings.TrimSpace(sm.Unit)
			if sm.AssetID == "" || sm.Metric == "" || sm.Unit == "" {
				return fmt.Errorf("asset metric sample requires non-empty asset_id, metric, and unit")
			}
		}
		if sm.CollectedAt.IsZero() {
			sm.CollectedAt = now
		} else {
			sm.CollectedAt = sm.CollectedAt.UTC()
		}
		if sm.Scope != "" {
			hubSamples = append(hubSamples, sm)
		} else {
			assetSamples = append(assetSamples, sm)
		}
	}
	if len(assetSamples) == 0 && len(hubSamples) == 0 {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	missingAssetIDs := make(map[string]struct{})
	var skippedSamples int64
	for start := 0; start < len(assetSamples); start += maxMetricSamplesPerInsert {
		end := min(start+maxMetricSamplesPerInsert, len(assetSamples))
		chunkErr := s.appendAssetMetricSamples(ctx, tx, assetSamples[start:end])
		if chunkErr == nil {
			continue
		}
		var unknownAssetsErr *UnknownMetricAssetsError
		if !errors.As(chunkErr, &unknownAssetsErr) {
			return chunkErr
		}
		skippedSamples += unknownAssetsErr.SkippedSamples
		for _, assetID := range unknownAssetsErr.AssetIDs {
			missingAssetIDs[assetID] = struct{}{}
		}
	}
	writtenHubScopes := make(map[string]struct{}, len(incomingHubSeries))
	for _, sample := range hubSamples {
		writtenHubScopes[sample.Scope] = struct{}{}
	}
	hubScopes := make([]string, 0, len(writtenHubScopes))
	for scope := range writtenHubScopes {
		hubScopes = append(hubScopes, scope)
	}
	sort.Strings(hubScopes)
	for _, scope := range hubScopes {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "labtether:hub-metrics:"+scope); err != nil {
			return err
		}
	}
	for start := 0; start < len(hubSamples); start += maxMetricSamplesPerInsert {
		end := min(start+maxMetricSamplesPerInsert, len(hubSamples))
		if err := s.appendHubMetricSamples(ctx, tx, hubSamples[start:end]); err != nil {
			return err
		}
	}
	for _, scope := range hubScopes {
		if err := s.compactHubMetricScope(ctx, tx, scope); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if skippedSamples > 0 {
		ids := make([]string, 0, len(missingAssetIDs))
		for assetID := range missingAssetIDs {
			ids = append(ids, assetID)
		}
		sort.Strings(ids)
		return &UnknownMetricAssetsError{AssetIDs: ids, SkippedSamples: skippedSamples}
	}
	return nil
}

// UnknownMetricAssetsError reports a partial telemetry write. Samples for
// existing assets were persisted, while samples for the listed missing assets
// were rejected. Callers must surface this error so stale producers remain
// observable instead of silently losing data.
type UnknownMetricAssetsError struct {
	AssetIDs       []string
	SkippedSamples int64
}

func (e *UnknownMetricAssetsError) Error() string {
	if e == nil {
		return ""
	}
	ids := e.AssetIDs
	const maxReportedAssetIDs = 8
	if len(ids) > maxReportedAssetIDs {
		ids = ids[:maxReportedAssetIDs]
	}
	return fmt.Sprintf("skipped %d metric samples for %d unknown assets: %q", e.SkippedSamples, len(e.AssetIDs), ids)
}

// appendAssetMetricSamples inserts samples for assets that exist in the same
// statement snapshot. The join prevents a missing/deleted asset from causing a
// batch FK violation (and an error-driven N+1 retry storm), while the result
// reports every skipped ID explicitly.
func (s *PostgresStore) appendAssetMetricSamples(ctx context.Context, tx pgx.Tx, samples []telemetry.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("WITH incoming (asset_id, metric, unit, value, collected_at, labels) AS (VALUES ")
	args := make([]any, 0, len(samples)*6)
	for i, sample := range samples {
		if i > 0 {
			b.WriteByte(',')
		}
		base := i * 6
		if i == 0 {
			fmt.Fprintf(&b, "($%d::text, $%d::text, $%d::text, $%d::double precision, $%d::timestamptz, $%d::jsonb)", base+1, base+2, base+3, base+4, base+5, base+6)
		} else {
			fmt.Fprintf(&b, "($%d, $%d, $%d, $%d, $%d, $%d::jsonb)", base+1, base+2, base+3, base+4, base+5, base+6)
		}
		labelsArg, err := labelsToJSONArg(sample.Labels)
		if err != nil {
			return fmt.Errorf("marshal labels for sample %q/%q: %w", sample.AssetID, sample.Metric, err)
		}
		args = append(args, sample.AssetID, sample.Metric, sample.Unit, sample.Value, sample.CollectedAt, labelsArg)
	}
	b.WriteString(`), inserted AS (
		INSERT INTO metric_samples (asset_id, metric, unit, value, collected_at, labels)
		SELECT incoming.asset_id, incoming.metric, incoming.unit, incoming.value, incoming.collected_at, incoming.labels
		  FROM incoming
		  JOIN assets ON assets.id = incoming.asset_id
		RETURNING 1
	)
	SELECT
		(SELECT COUNT(*) FROM inserted),
		(SELECT COUNT(*) FROM incoming WHERE NOT EXISTS (SELECT 1 FROM assets WHERE assets.id = incoming.asset_id)),
		COALESCE(
			(SELECT ARRAY_AGG(DISTINCT incoming.asset_id ORDER BY incoming.asset_id)
			   FROM incoming
			  WHERE NOT EXISTS (SELECT 1 FROM assets WHERE assets.id = incoming.asset_id)),
			ARRAY[]::text[]
		)`)

	var (
		insertedCount int64
		skippedCount  int64
		missingIDs    []string
	)
	if err := tx.QueryRow(ctx, b.String(), args...).Scan(&insertedCount, &skippedCount, &missingIDs); err != nil {
		return err
	}
	if skippedCount > 0 {
		return &UnknownMetricAssetsError{AssetIDs: missingIDs, SkippedSamples: skippedCount}
	}
	if insertedCount != int64(len(samples)) {
		return fmt.Errorf("metric sample insert count mismatch: inserted %d of %d", insertedCount, len(samples))
	}
	return nil
}

func (s *PostgresStore) appendHubMetricSamples(ctx context.Context, tx pgx.Tx, samples []telemetry.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("INSERT INTO hub_metric_samples (scope, metric, unit, value, collected_at, labels) VALUES ")
	args := make([]any, 0, len(samples)*6)
	for i, sample := range samples {
		if i > 0 {
			b.WriteByte(',')
		}
		base := i * 6
		fmt.Fprintf(&b, "($%d, $%d, $%d, $%d, $%d, $%d::jsonb)", base+1, base+2, base+3, base+4, base+5, base+6)
		labelsArg, err := labelsToJSONArg(sample.Labels)
		if err != nil {
			return fmt.Errorf("marshal labels for hub sample %q/%q: %w", sample.Scope, sample.Metric, err)
		}
		args = append(args, sample.Scope, sample.Metric, sample.Unit, sample.Value, sample.CollectedAt, labelsArg)
	}
	_, err := tx.Exec(ctx, b.String(), args...)
	return err
}

// compactHubMetricScope applies the same event-time policy as the memory
// store: keep the 1,024 freshest distinct series and the 16 freshest history
// rows per retained series. Late old batches cannot evict fresher series.
func (s *PostgresStore) compactHubMetricScope(ctx context.Context, tx pgx.Tx, scope string) error {
	_, err := tx.Exec(ctx, `
		WITH latest_series AS MATERIALIZED (
			SELECT DISTINCT ON (metric, COALESCE(labels, '{}'::jsonb))
			       metric,
			       COALESCE(labels, '{}'::jsonb) AS labels_key,
			       collected_at,
			       id
			  FROM hub_metric_samples
			 WHERE scope = $1
			 ORDER BY metric, COALESCE(labels, '{}'::jsonb), collected_at DESC, id DESC
		), kept_series AS MATERIALIZED (
			SELECT metric, labels_key
			  FROM latest_series
			 ORDER BY collected_at DESC, id DESC, metric, labels_key
			 LIMIT $2
		), ranked_rows AS MATERIALIZED (
			SELECT sample.id,
			       ROW_NUMBER() OVER (
				   PARTITION BY sample.metric, COALESCE(sample.labels, '{}'::jsonb)
				   ORDER BY sample.collected_at DESC, sample.id DESC
			       ) AS history_rank,
			       EXISTS (
				   SELECT 1
				     FROM kept_series
				    WHERE kept_series.metric = sample.metric
				      AND kept_series.labels_key = COALESCE(sample.labels, '{}'::jsonb)
			       ) AS keep_series
			  FROM hub_metric_samples AS sample
			 WHERE sample.scope = $1
		)
		DELETE FROM hub_metric_samples AS sample
		 USING ranked_rows
		 WHERE sample.id = ranked_rows.id
		   AND (NOT ranked_rows.keep_series OR ranked_rows.history_rank > $3)`,
		scope,
		telemetry.MaxHubMetricSeriesPerScope,
		telemetry.MaxHubMetricHistoryPerSeries,
	)
	return err
}

// labelsToJSONArg converts a labels map to a JSONB-compatible argument.
// Returns nil (SQL NULL) for nil/empty maps, JSON string for populated maps.
// NOTE: Does not use marshalStringMap because that returns "{}" for empty maps,
// but we need NULL for unlabeled samples to preserve backward compatibility.
func labelsToJSONArg(labels map[string]string) (any, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}
