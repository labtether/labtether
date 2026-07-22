package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/telemetry"
)

var (
	canonicalTelemetryMetricNamesOnce   sync.Once
	canonicalTelemetryMetricNamesCached []string
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
	rows, err := s.pool.Query(context.Background(),
		`SELECT DISTINCT ON (metric) metric, value
		 FROM metric_samples
		 WHERE asset_id = $1
		   AND collected_at <= $2
		 ORDER BY metric, collected_at DESC`,
		assetID,
		at.UTC(),
	)
	if err != nil {
		return telemetry.DynamicSnapshot{}, err
	}
	defer rows.Close()

	metrics := make(map[string]float64)
	for rows.Next() {
		var metric string
		var value float64
		if err := rows.Scan(&metric, &value); err != nil {
			return telemetry.DynamicSnapshot{}, err
		}
		metrics[metric] = value
	}
	if rows.Err() != nil {
		return telemetry.DynamicSnapshot{}, rows.Err()
	}
	return telemetry.DynamicSnapshot{Metrics: metrics}, nil
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

	// Query the latest value per (asset, metric) across all metrics via
	// DISTINCT ON. No metric name filter — returns whatever metrics exist.
	rows, err := s.pool.Query(context.Background(),
		`SELECT DISTINCT ON (asset_id, metric) asset_id, metric, value
		 FROM metric_samples
		 WHERE asset_id = ANY($1::text[])
		   AND collected_at <= $2
		 ORDER BY asset_id, metric, collected_at DESC`,
		cleanIDs,
		at.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			assetID string
			metric  string
			value   float64
		)
		if err := rows.Scan(&assetID, &metric, &value); err != nil {
			return nil, err
		}
		dyn, ok := out[assetID]
		if !ok {
			continue
		}
		if dyn.Metrics == nil {
			dyn.Metrics = make(map[string]float64)
		}
		dyn.Metrics[metric] = value
		out[assetID] = dyn
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return out, nil
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

func (s *PostgresStore) AppendEvent(event logs.Event) error {
	normalized, fieldsPayload, _, err := normalizeLogEventForInsert(event)
	if err != nil {
		return err
	}

	result, err := s.pool.Exec(context.Background(),
		`INSERT INTO log_events (id, asset_id, source, level, message, fields, timestamp)
			 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
			 ON CONFLICT (id) DO NOTHING`,
		normalized.ID,
		nullIfBlank(normalized.AssetID),
		normalized.Source,
		normalized.Level,
		normalized.Message,
		fieldsPayload,
		normalized.Timestamp.UTC(),
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() > 0 {
		s.updateLogEventsWatermarkCache(normalized.Timestamp.UTC(), time.Now().UTC())
	}
	return nil
}

func (s *PostgresStore) AppendEvents(events []logs.Event) error {
	if len(events) == 0 {
		return nil
	}

	normalized, payloads, err := normalizeLogEventsForInsert(events)
	if err != nil {
		return err
	}
	latest := time.Unix(0, 0).UTC()
	for _, event := range normalized {
		if event.Timestamp.After(latest) {
			latest = event.Timestamp.UTC()
		}
	}
	if len(normalized) == 0 {
		return nil
	}

	const columnsPerRow = 7
	args := make([]any, 0, len(normalized)*columnsPerRow)
	var query strings.Builder
	query.Grow(128 + len(normalized)*64)
	query.WriteString(`INSERT INTO log_events (id, asset_id, source, level, message, fields, timestamp) VALUES `)
	for idx, event := range normalized {
		if idx > 0 {
			query.WriteByte(',')
		}
		base := idx*columnsPerRow + 1
		query.WriteByte('(')
		for col := 0; col < columnsPerRow; col++ {
			if col > 0 {
				query.WriteByte(',')
			}
			query.WriteByte('$')
			query.WriteString(strconv.Itoa(base + col))
			if col == 5 {
				query.WriteString("::jsonb")
			}
		}
		query.WriteByte(')')
		args = append(args,
			event.ID,
			nullIfBlank(event.AssetID),
			event.Source,
			event.Level,
			event.Message,
			payloads[idx],
			event.Timestamp.UTC(),
		)
	}
	query.WriteString(` ON CONFLICT (id) DO NOTHING`)

	result, err := s.pool.Exec(context.Background(), query.String(), args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() > 0 {
		s.updateLogEventsWatermarkCache(latest, time.Now().UTC())
	}
	return nil
}

func normalizeLogEventForInsert(event logs.Event) (logs.Event, string, int, error) {
	if event.ID == "" {
		event.ID = idgen.New("log")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if strings.TrimSpace(event.Source) == "" {
		event.Source = "labtether"
	}
	if strings.TrimSpace(event.Level) == "" {
		event.Level = "info"
	}
	if strings.TrimSpace(event.Message) == "" {
		event.Message = "event"
	}
	event.Level = strings.ToLower(strings.TrimSpace(event.Level))
	eventBytes, err := logs.EventEnvelopeBytes(event)
	if err != nil {
		return logs.Event{}, "", 0, err
	}
	event.Fields = cloneMetadata(event.Fields)

	fieldsPayload, err := marshalStringMap(event.Fields)
	if err != nil {
		return logs.Event{}, "", 0, err
	}
	return event, fieldsPayload, eventBytes, nil
}

func (s *PostgresStore) QueryEvents(req logs.QueryRequest) ([]logs.Event, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	from := req.From.UTC()
	to := req.To.UTC()
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}

	fieldKeys := normalizeLogFieldKeys(req.FieldKeys)
	groupID := strings.TrimSpace(req.GroupID)
	groupAssetIDs := normalizeLogAssetIDs(req.GroupAssetIDs)

	args := make([]any, 0, 10)
	where := make([]string, 0, 8)
	next := 1

	where = append(where, fmt.Sprintf("timestamp >= $%d", next))
	args = append(args, from)
	next++
	where = append(where, fmt.Sprintf("timestamp <= $%d", next))
	args = append(args, to)
	next++

	if assetID := strings.TrimSpace(req.AssetID); assetID != "" {
		where = append(where, fmt.Sprintf("asset_id = $%d", next))
		args = append(args, assetID)
		next++
	}
	if source := strings.TrimSpace(req.Source); source != "" {
		where = append(where, fmt.Sprintf("source = $%d", next))
		args = append(args, source)
		next++
	}
	if level := strings.TrimSpace(req.Level); level != "" {
		where = append(where, fmt.Sprintf("level = $%d", next))
		args = append(args, strings.ToLower(level))
		next++
	}
	if groupID != "" {
		if len(groupAssetIDs) > 0 {
			where = append(where, fmt.Sprintf("(asset_id = ANY($%d::text[]) OR NULLIF(BTRIM(fields->>'group_id'), '') = $%d)", next, next+1))
			args = append(args, groupAssetIDs, groupID)
			next += 2
		} else {
			where = append(where, fmt.Sprintf("NULLIF(BTRIM(fields->>'group_id'), '') = $%d", next))
			args = append(args, groupID)
			next++
		}
	} else if len(groupAssetIDs) > 0 {
		where = append(where, fmt.Sprintf("asset_id = ANY($%d::text[])", next))
		args = append(args, groupAssetIDs)
		next++
	}
	if search := strings.TrimSpace(req.Search); search != "" {
		where = append(where, fmt.Sprintf("(message ILIKE $%d OR source ILIKE $%d)", next, next))
		args = append(args, "%"+search+"%")
		next++
	}

	fieldsProjection := "fields"
	scanProjectedGroupID := false
	if req.ExcludeFields {
		fieldsProjection = "NULL::jsonb AS fields"
	} else if len(fieldKeys) == 1 && fieldKeys[0] == "group_id" {
		fieldsProjection = "NULL::jsonb AS fields, CASE WHEN asset_id IS NULL THEN COALESCE(NULLIF(BTRIM(fields->>'group_id'), ''), '') ELSE '' END AS projected_group_id"
		scanProjectedGroupID = true
	} else if len(fieldKeys) > 0 {
		pairs := make([]string, 0, len(fieldKeys))
		for _, key := range fieldKeys {
			pairs = append(pairs, fmt.Sprintf("$%d::text, fields->>($%d::text)", next, next))
			args = append(args, key)
			next++
		}
		fieldsProjection = "jsonb_strip_nulls(jsonb_build_object(" + strings.Join(pairs, ", ") + ")) AS fields"
	}

	sql := `SELECT id, asset_id, source, level, message, ` + fieldsProjection + `, timestamp
		FROM log_events`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY timestamp DESC LIMIT $%d", next)
	args = append(args, limit)

	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]logs.Event, 0)
	for rows.Next() {
		event := logs.Event{}
		var assetID *string
		var fields []byte
		if scanProjectedGroupID {
			var projectedGroupID string
			if err := rows.Scan(
				&event.ID,
				&assetID,
				&event.Source,
				&event.Level,
				&event.Message,
				&fields,
				&projectedGroupID,
				&event.Timestamp,
			); err != nil {
				return nil, err
			}
			if projectedGroupID = strings.TrimSpace(projectedGroupID); projectedGroupID != "" {
				event.Fields = map[string]string{"group_id": projectedGroupID}
			}
		} else {
			if err := rows.Scan(
				&event.ID,
				&assetID,
				&event.Source,
				&event.Level,
				&event.Message,
				&fields,
				&event.Timestamp,
			); err != nil {
				return nil, err
			}
		}
		if assetID != nil {
			event.AssetID = *assetID
		}
		if !scanProjectedGroupID && !req.ExcludeFields && len(fields) > 0 {
			parsed := map[string]string{}
			if err := json.Unmarshal(fields, &parsed); err == nil && len(parsed) > 0 {
				event.Fields = parsed
			}
		}
		out = append(out, event)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) QueryDeadLetterEvents(from, to time.Time, limit int) ([]logs.DeadLetterEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	from = from.UTC()
	to = to.UTC()
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT
			COALESCE(NULLIF(BTRIM(fields->>'event_id'), ''), id) AS event_id,
			COALESCE(BTRIM(fields->>'component'), '') AS component,
			COALESCE(BTRIM(fields->>'subject'), '') AS subject,
			COALESCE(NULLIF(BTRIM(fields->>'deliveries'), ''), '0') AS deliveries,
			COALESCE(NULLIF(BTRIM(fields->>'error'), ''), BTRIM(message)) AS error_message,
			COALESCE(BTRIM(fields->>'payload_b64'), '') AS payload_b64,
			timestamp
		FROM log_events
		WHERE source = 'dead_letter'
		  AND level = 'error'
		  AND timestamp >= $1
		  AND timestamp <= $2
		ORDER BY timestamp DESC
		LIMIT $3`,
		from,
		to,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]logs.DeadLetterEvent, 0)
	for rows.Next() {
		var entry logs.DeadLetterEvent
		var deliveriesRaw string
		if err := rows.Scan(
			&entry.ID,
			&entry.Component,
			&entry.Subject,
			&deliveriesRaw,
			&entry.Error,
			&entry.PayloadB64,
			&entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		if parsed, err := strconv.ParseUint(strings.TrimSpace(deliveriesRaw), 10, 64); err == nil {
			entry.Deliveries = parsed
		}
		entry.CreatedAt = entry.CreatedAt.UTC()
		out = append(out, entry)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return out, nil
}

func (s *PostgresStore) CountDeadLetterEvents(from, to time.Time) (int, error) {
	from = from.UTC()
	to = to.UTC()
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}

	var total int64
	if err := s.pool.QueryRow(context.Background(),
		`SELECT COUNT(*)
		 FROM log_events
		 WHERE source = 'dead_letter'
		   AND level = 'error'
		   AND timestamp >= $1
		   AND timestamp <= $2`,
		from,
		to,
	).Scan(&total); err != nil {
		return 0, err
	}
	if total < 0 {
		total = 0
	}
	return int(total), nil
}

func (s *PostgresStore) ListSourcesSince(limit int, from time.Time) ([]logs.SourceSummary, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT source, COUNT(*) AS event_count, MAX(timestamp) AS last_seen
		 FROM log_events
		 WHERE timestamp >= $2
		 GROUP BY source
		 ORDER BY last_seen DESC
		 LIMIT $1`,
		limit,
		from.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]logs.SourceSummary, 0)
	for rows.Next() {
		row := logs.SourceSummary{}
		if err := rows.Scan(&row.Source, &row.Count, &row.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) ListSources(limit int) ([]logs.SourceSummary, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT source, COUNT(*) AS event_count, MAX(timestamp) AS last_seen
		 FROM log_events
		 WHERE timestamp >= NOW() - INTERVAL '24 hours'
		 GROUP BY source
		 ORDER BY last_seen DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]logs.SourceSummary, 0)
	for rows.Next() {
		row := logs.SourceSummary{}
		if err := rows.Scan(&row.Source, &row.Count, &row.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) QuerySourceSummaries(req logs.SourceSummaryRequest) ([]logs.SourceSummary, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	from := req.From.UTC()
	to := req.To.UTC()
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}

	groupID := strings.TrimSpace(req.GroupID)
	groupAssetIDs := normalizeLogAssetIDs(req.GroupAssetIDs)

	args := make([]any, 0, 6)
	where := make([]string, 0, 4)
	next := 1

	where = append(where, fmt.Sprintf("timestamp >= $%d", next))
	args = append(args, from)
	next++
	where = append(where, fmt.Sprintf("timestamp <= $%d", next))
	args = append(args, to)
	next++

	if groupID != "" {
		if len(groupAssetIDs) > 0 {
			where = append(where, fmt.Sprintf("(asset_id = ANY($%d::text[]) OR NULLIF(BTRIM(fields->>'group_id'), '') = $%d)", next, next+1))
			args = append(args, groupAssetIDs, groupID)
			next += 2
		} else {
			where = append(where, fmt.Sprintf("NULLIF(BTRIM(fields->>'group_id'), '') = $%d", next))
			args = append(args, groupID)
			next++
		}
	} else if len(groupAssetIDs) > 0 {
		where = append(where, fmt.Sprintf("asset_id = ANY($%d::text[])", next))
		args = append(args, groupAssetIDs)
		next++
	}

	sql := `SELECT source, COUNT(*) AS event_count, MAX(timestamp) AS last_seen
		FROM log_events`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" GROUP BY source ORDER BY last_seen DESC LIMIT $%d", next)
	args = append(args, limit)

	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]logs.SourceSummary, 0)
	for rows.Next() {
		row := logs.SourceSummary{}
		if err := rows.Scan(&row.Source, &row.Count, &row.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) QueryGroupSeverityCounts(req logs.GroupSeverityCountRequest) ([]logs.GroupSeverityCount, error) {
	from := req.From.UTC()
	to := req.To.UTC()
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}

	assetIDs := make([]string, 0, len(req.AssetGroups))
	groupIDsByAsset := make([]string, 0, len(req.AssetGroups))
	for assetID, groupID := range req.AssetGroups {
		assetID = strings.TrimSpace(assetID)
		groupID = strings.TrimSpace(groupID)
		if assetID == "" || groupID == "" {
			continue
		}
		assetIDs = append(assetIDs, assetID)
		groupIDsByAsset = append(groupIDsByAsset, groupID)
	}
	filterGroupIDs := make([]string, 0, len(req.GroupIDs))
	for _, groupID := range req.GroupIDs {
		groupID = strings.TrimSpace(groupID)
		if groupID == "" {
			continue
		}
		filterGroupIDs = append(filterGroupIDs, groupID)
	}

	rows, err := s.pool.Query(context.Background(),
		`WITH asset_groups AS (
			SELECT *
			FROM unnest($3::text[], $4::text[]) AS ag(asset_id, group_id)
		)
		SELECT
			resolved.group_id,
			COALESCE(SUM(CASE WHEN resolved.level = 'error' THEN 1 ELSE 0 END), 0) AS error_count,
			COALESCE(SUM(CASE WHEN resolved.level IN ('warn', 'warning') THEN 1 ELSE 0 END), 0) AS warn_count,
			COALESCE(SUM(CASE WHEN resolved.level = 'error' AND resolved.source = 'dead_letter' THEN 1 ELSE 0 END), 0) AS dead_letter_count
		FROM (
			SELECT
				COALESCE(NULLIF(BTRIM(le.fields->>'group_id'), ''), ag.group_id) AS group_id,
				LOWER(BTRIM(le.level)) AS level,
				BTRIM(le.source) AS source
			FROM log_events le
			LEFT JOIN asset_groups ag ON ag.asset_id = le.asset_id
			WHERE le.timestamp >= $1
			  AND le.timestamp <= $2
			  AND COALESCE(NULLIF(BTRIM(le.fields->>'group_id'), ''), ag.group_id) <> ''
			  AND (cardinality($5::text[]) = 0 OR COALESCE(NULLIF(BTRIM(le.fields->>'group_id'), ''), ag.group_id) = ANY($5::text[]))
		) AS resolved
		GROUP BY resolved.group_id
		ORDER BY resolved.group_id ASC`,
		from,
		to,
		assetIDs,
		groupIDsByAsset,
		filterGroupIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]logs.GroupSeverityCount, 0, 16)
	for rows.Next() {
		entry := logs.GroupSeverityCount{}
		if err := rows.Scan(&entry.GroupID, &entry.ErrorCount, &entry.WarnCount, &entry.DeadLetterCount); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) LogEventsWatermark() (time.Time, error) {
	now := time.Now().UTC()
	if watermark, ok := s.cachedLogEventsWatermark(now); ok {
		return watermark, nil
	}

	var watermark time.Time
	if err := s.pool.QueryRow(
		context.Background(),
		`SELECT COALESCE(MAX(timestamp), to_timestamp(0)) FROM log_events`,
	).Scan(&watermark); err != nil {
		return time.Time{}, err
	}
	watermark = watermark.UTC()
	s.updateLogEventsWatermarkCache(watermark, now)
	return watermark, nil
}

func (s *PostgresStore) cachedLogEventsWatermark(now time.Time) (time.Time, bool) {
	s.logEventsWatermarkMu.RLock()
	watermark := s.logEventsWatermark.UTC()
	fetchedAt := s.logEventsWatermarkFetchedAt.UTC()
	refreshInterval := s.logEventsWatermarkRefreshInterval
	s.logEventsWatermarkMu.RUnlock()

	if refreshInterval <= 0 {
		refreshInterval = defaultLogEventsWatermarkRefreshInterval
	}
	if watermark.IsZero() || fetchedAt.IsZero() {
		return time.Time{}, false
	}
	if now.Sub(fetchedAt) <= refreshInterval {
		return watermark, true
	}
	return time.Time{}, false
}

func (s *PostgresStore) updateLogEventsWatermarkCache(watermark, fetchedAt time.Time) {
	watermark = watermark.UTC()
	fetchedAt = fetchedAt.UTC()

	s.logEventsWatermarkMu.Lock()
	if watermark.After(s.logEventsWatermark) {
		s.logEventsWatermark = watermark
	}
	if s.logEventsWatermark.IsZero() {
		s.logEventsWatermark = watermark
	}
	if fetchedAt.After(s.logEventsWatermarkFetchedAt) {
		s.logEventsWatermarkFetchedAt = fetchedAt
	}
	if s.logEventsWatermarkRefreshInterval <= 0 {
		s.logEventsWatermarkRefreshInterval = defaultLogEventsWatermarkRefreshInterval
	}
	s.logEventsWatermarkMu.Unlock()
}

func (s *PostgresStore) invalidateLogEventsWatermarkCache() {
	s.logEventsWatermarkMu.Lock()
	s.logEventsWatermark = time.Unix(0, 0).UTC()
	s.logEventsWatermarkFetchedAt = time.Unix(0, 0).UTC()
	s.logEventsWatermarkMu.Unlock()
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

func (s *PostgresStore) SaveView(actorID string, req logs.SavedViewRequest) (logs.SavedView, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = idgen.New("view")
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}

	row := s.pool.QueryRow(context.Background(),
		`INSERT INTO saved_log_views (id, owner_id, name, asset_id, source, level, search, window_value, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		 RETURNING id, name, asset_id, source, level, search, window_value, created_at, updated_at`,
		id,
		actorID,
		strings.TrimSpace(req.Name),
		nullIfBlank(req.AssetID),
		nullIfBlank(req.Source),
		nullIfBlank(strings.ToLower(req.Level)),
		nullIfBlank(req.Search),
		nullIfBlank(req.Window),
		now,
	)

	view := logs.SavedView{}
	var assetID *string
	var source *string
	var level *string
	var search *string
	var window *string
	if err := row.Scan(
		&view.ID,
		&view.Name,
		&assetID,
		&source,
		&level,
		&search,
		&window,
		&view.CreatedAt,
		&view.UpdatedAt,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return logs.SavedView{}, ErrAlreadyExists
		}
		return logs.SavedView{}, err
	}
	if assetID != nil {
		view.AssetID = *assetID
	}
	if source != nil {
		view.Source = *source
	}
	if level != nil {
		view.Level = *level
	}
	if search != nil {
		view.Search = *search
	}
	if window != nil {
		view.Window = *window
	}

	return view, nil
}

func (s *PostgresStore) ListViews(actorID string, limit int) ([]logs.SavedView, error) {
	if limit <= 0 {
		limit = 50
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, name, asset_id, source, level, search, window_value, created_at, updated_at
		 FROM saved_log_views
		 WHERE owner_id = $1
		 ORDER BY updated_at DESC
		 LIMIT $2`,
		actorID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]logs.SavedView, 0)
	for rows.Next() {
		view := logs.SavedView{}
		var assetID *string
		var source *string
		var level *string
		var search *string
		var window *string
		if err := rows.Scan(
			&view.ID,
			&view.Name,
			&assetID,
			&source,
			&level,
			&search,
			&window,
			&view.CreatedAt,
			&view.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if assetID != nil {
			view.AssetID = *assetID
		}
		if source != nil {
			view.Source = *source
		}
		if level != nil {
			view.Level = *level
		}
		if search != nil {
			view.Search = *search
		}
		if window != nil {
			view.Window = *window
		}
		out = append(out, view)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) GetView(actorID, id string) (logs.SavedView, bool, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, name, asset_id, source, level, search, window_value, created_at, updated_at
		 FROM saved_log_views
		 WHERE owner_id = $1 AND id = $2`,
		actorID,
		strings.TrimSpace(id),
	)

	view := logs.SavedView{}
	var assetID *string
	var source *string
	var level *string
	var search *string
	var window *string
	if err := row.Scan(
		&view.ID,
		&view.Name,
		&assetID,
		&source,
		&level,
		&search,
		&window,
		&view.CreatedAt,
		&view.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return logs.SavedView{}, false, nil
		}
		return logs.SavedView{}, false, err
	}
	if assetID != nil {
		view.AssetID = *assetID
	}
	if source != nil {
		view.Source = *source
	}
	if level != nil {
		view.Level = *level
	}
	if search != nil {
		view.Search = *search
	}
	if window != nil {
		view.Window = *window
	}
	return view, true, nil
}

func (s *PostgresStore) UpdateView(actorID, id string, req logs.SavedViewRequest) (logs.SavedView, error) {
	now := time.Now().UTC()
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	id = strings.TrimSpace(id)

	row := s.pool.QueryRow(context.Background(),
		`UPDATE saved_log_views
		 SET name = $3, asset_id = $4, source = $5, level = $6, search = $7, window_value = $8, updated_at = $9
		 WHERE owner_id = $1 AND id = $2
		 RETURNING id, name, asset_id, source, level, search, window_value, created_at, updated_at`,
		actorID,
		id,
		strings.TrimSpace(req.Name),
		nullIfBlank(req.AssetID),
		nullIfBlank(req.Source),
		nullIfBlank(strings.ToLower(req.Level)),
		nullIfBlank(req.Search),
		nullIfBlank(req.Window),
		now,
	)

	view := logs.SavedView{}
	var viewAssetID *string
	var viewSource *string
	var viewLevel *string
	var viewSearch *string
	var viewWindow *string
	if err := row.Scan(
		&view.ID,
		&view.Name,
		&viewAssetID,
		&viewSource,
		&viewLevel,
		&viewSearch,
		&viewWindow,
		&view.CreatedAt,
		&view.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return logs.SavedView{}, ErrNotFound
		}
		return logs.SavedView{}, err
	}
	if viewAssetID != nil {
		view.AssetID = *viewAssetID
	}
	if viewSource != nil {
		view.Source = *viewSource
	}
	if viewLevel != nil {
		view.Level = *viewLevel
	}
	if viewSearch != nil {
		view.Search = *viewSearch
	}
	if viewWindow != nil {
		view.Window = *viewWindow
	}
	return view, nil
}

func (s *PostgresStore) DeleteView(actorID, id string) error {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM saved_log_views WHERE owner_id = $1 AND id = $2`,
		actorID,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
