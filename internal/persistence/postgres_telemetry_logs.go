package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/telemetry"
)

var (
	canonicalTelemetryMetricNamesOnce   sync.Once
	canonicalTelemetryMetricNamesCached []string
)

func (s *PostgresStore) AppendSamples(ctx context.Context, samples []telemetry.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}

	var valid []telemetry.MetricSample
	for _, sample := range samples {
		if sample.AssetID == "" || sample.Metric == "" || sample.Unit == "" {
			continue
		}
		sm := sample
		if sm.CollectedAt.IsZero() {
			sm.CollectedAt = time.Now().UTC()
		} else {
			sm.CollectedAt = sm.CollectedAt.UTC()
		}
		valid = append(valid, sm)
	}
	if len(valid) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("INSERT INTO metric_samples (asset_id, metric, unit, value, collected_at, labels) VALUES ")
	args := make([]any, 0, len(valid)*6)
	for i, sample := range valid {
		if i > 0 {
			b.WriteByte(',')
		}
		base := i * 6
		fmt.Fprintf(&b, "($%d, $%d, $%d, $%d, $%d, $%d::jsonb)", base+1, base+2, base+3, base+4, base+5, base+6)
		labelsArg, err := labelsToJSONArg(sample.Labels)
		if err != nil {
			return fmt.Errorf("marshal labels for sample %q/%q: %w", sample.AssetID, sample.Metric, err)
		}
		args = append(args, sample.AssetID, sample.Metric, sample.Unit, sample.Value, sample.CollectedAt, labelsArg)
	}

	_, err := s.pool.Exec(ctx, b.String(), args...)
	if err == nil {
		return nil
	}

	// If the batch fails (typically FK violation when a bridge emits samples
	// for asset IDs not yet registered), retry each sample individually so
	// valid samples are still persisted and only unknown asset IDs are skipped.
	if !isFKViolation(err) {
		return err
	}
	var firstErr error
	for _, sample := range valid {
		labelsArg, _ := labelsToJSONArg(sample.Labels)
		_, sErr := s.pool.Exec(ctx,
			"INSERT INTO metric_samples (asset_id, metric, unit, value, collected_at, labels) VALUES ($1, $2, $3, $4, $5, $6::jsonb)",
			sample.AssetID, sample.Metric, sample.Unit, sample.Value, sample.CollectedAt, labelsArg,
		)
		if sErr != nil && firstErr == nil && !isFKViolation(sErr) {
			firstErr = sErr
		}
	}
	return firstErr
}

// isFKViolation returns true if the error is a Postgres foreign key constraint
// violation (SQLSTATE 23503). Used to gracefully skip metric samples whose
// asset_id has not yet been registered by the discovery cycle.
func isFKViolation(err error) bool {
	if err == nil {
		return false
	}
	// pgx wraps the SQLSTATE in the error message. Check for the code directly.
	return strings.Contains(err.Error(), "SQLSTATE 23503") ||
		strings.Contains(err.Error(), "violates foreign key constraint")
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

	rows, err := s.pool.Query(context.Background(),
		`SELECT asset_id, collected_at, value
		 FROM metric_samples
		 WHERE asset_id = ANY($1::text[])
		   AND metric = $2
		   AND collected_at >= $3
		   AND collected_at <= $4
		 ORDER BY asset_id ASC, collected_at ASC`,
		cleanIDs,
		metric,
		start.UTC(),
		end.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rawPoints := make(map[string][]telemetry.Point, len(cleanIDs))
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
	normalized, fieldsPayload, err := normalizeLogEventForInsert(event)
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

	normalized := make([]logs.Event, 0, len(events))
	payloads := make([]string, 0, len(events))
	latest := time.Unix(0, 0).UTC()
	for _, event := range events {
		normalizedEvent, fieldsPayload, err := normalizeLogEventForInsert(event)
		if err != nil {
			return err
		}
		normalized = append(normalized, normalizedEvent)
		payloads = append(payloads, fieldsPayload)
		if normalizedEvent.Timestamp.After(latest) {
			latest = normalizedEvent.Timestamp.UTC()
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

func normalizeLogEventForInsert(event logs.Event) (logs.Event, string, error) {
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

	fieldsPayload, err := marshalStringMap(event.Fields)
	if err != nil {
		return logs.Event{}, "", err
	}
	return event, fieldsPayload, nil
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

	out := make([]logs.Event, 0, limit)
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

	out := make([]logs.DeadLetterEvent, 0, limit)
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

	out := make([]logs.SourceSummary, 0, limit)
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

	out := make([]logs.SourceSummary, 0, limit)
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
		`SELECT COALESCE(MAX(collected_at), to_timestamp(0)) FROM metric_samples`,
	).Scan(&watermark); err != nil {
		return time.Time{}, err
	}
	return watermark.UTC(), nil
}

func (s *PostgresStore) SaveView(req logs.SavedViewRequest) (logs.SavedView, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = idgen.New("view")
	}

	row := s.pool.QueryRow(context.Background(),
		`INSERT INTO saved_log_views (id, name, asset_id, source, level, search, window_value, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		 ON CONFLICT (id) DO UPDATE
		 SET name = EXCLUDED.name,
		     asset_id = EXCLUDED.asset_id,
		     source = EXCLUDED.source,
		     level = EXCLUDED.level,
		     search = EXCLUDED.search,
		     window_value = EXCLUDED.window_value,
		     updated_at = EXCLUDED.updated_at
		 RETURNING id, name, asset_id, source, level, search, window_value, created_at, updated_at`,
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

func (s *PostgresStore) ListViews(limit int) ([]logs.SavedView, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, name, asset_id, source, level, search, window_value, created_at, updated_at
		 FROM saved_log_views
		 ORDER BY updated_at DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]logs.SavedView, 0, limit)
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

func (s *PostgresStore) GetView(id string) (logs.SavedView, bool, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, name, asset_id, source, level, search, window_value, created_at, updated_at
		 FROM saved_log_views
		 WHERE id = $1`,
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

func (s *PostgresStore) UpdateView(id string, req logs.SavedViewRequest) (logs.SavedView, error) {
	now := time.Now().UTC()
	id = strings.TrimSpace(id)

	row := s.pool.QueryRow(context.Background(),
		`UPDATE saved_log_views
		 SET name = $2, asset_id = $3, source = $4, level = $5, search = $6, window_value = $7, updated_at = $8
		 WHERE id = $1
		 RETURNING id, name, asset_id, source, level, search, window_value, created_at, updated_at`,
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

func (s *PostgresStore) DeleteView(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM saved_log_views WHERE id = $1`,
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
