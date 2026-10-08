package persistence

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/logs"
	"strings"
	"time"
)

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
