package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/logs"
	"strconv"
	"strings"
	"time"
)

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
