package persistence

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/alerts"
)

// Reject bad stored scope before LIMIT so it cannot occupy a whole page.
// NormalizeFiringScope remains the final check on decoded evidence.
func correlationValidScopeSQL() string {
	parts := []string{`jsonb_typeof(s.firing_scope)='object'`, `jsonb_typeof(s.firing_scope->'version')='number' AND s.firing_scope->>'version'='1'`}
	for _, key := range []string{"asset_ids", "group_ids", "synthetic_check_ids"} {
		field := "s.firing_scope->'" + key + "'"
		parts = append(parts, `(`+field+` IS NULL OR `+field+`='null'::jsonb OR (jsonb_typeof(`+field+`)='array' AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(`+field+`)='array' THEN `+field+` ELSE '[]'::jsonb END) v WHERE jsonb_typeof(v)<>'string' OR (v#>>'{}')='' OR btrim(v#>>'{}', `+correlationTrimSpaceSQL+`)<>(v#>>'{}'))))`)
	}
	return "(s.firing_scope IS NULL OR (" + strings.Join(parts, " AND ") + "))"
}

// strings.TrimSpace's Unicode White_Space set, independent of DB collation.
const correlationTrimSpaceSQL = `U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'`

// Only witnesses relevant to this incident leave the store. The WHERE clause
// validates the complete saved scope before this bounded projection is read.
const correlationAlertWitnessSQL = `CASE WHEN s.firing_scope IS NULL THEN NULL ELSE
 jsonb_build_object('version',1,'asset_ids',COALESCE((SELECT jsonb_agg(a.id) FROM
  (SELECT v#>>'{}' AS id FROM jsonb_array_elements(CASE WHEN jsonb_typeof(s.firing_scope->'asset_ids')='array' THEN s.firing_scope->'asset_ids' ELSE '[]'::jsonb END) v
   WHERE (v#>>'{}')=ANY($3::text[]) ORDER BY (v#>>'{}') COLLATE "C" LIMIT 1) a),'[]'::jsonb),
 'group_ids',CASE WHEN $4<>'' AND (s.firing_scope->'group_ids') ? $4 THEN jsonb_build_array($4::text) ELSE '[]'::jsonb END) END`

func (s *PostgresStore) ReadAlertCorrelationPage(ctx context.Context, w CorrelationWindow, scope IncidentCorrelationScope, cursor CorrelationCursor) ([]CorrelationAlert, CorrelationCursor, bool, error) {
	if err := validateCorrelationQuery(w, scope); err != nil {
		return nil, cursor, false, err
	}
	links, err := json.Marshal(scope.AlertLinks)
	if err != nil {
		return nil, cursor, false, err
	}
	if scope.AlertLinks == nil {
		links = []byte("[]")
	}
	predicate := `s.first_fired_at IS NOT NULL AND s.first_fired_at >= $1 AND s.first_fired_at <= $2 AND ` + correlationValidScopeSQL() + ` AND (
 (s.firing_scope->'asset_ids') ?| $3::text[] OR ($4<>'' AND (s.firing_scope->'group_ids') ? $4) OR
 EXISTS (SELECT 1 FROM jsonb_array_elements($5::jsonb) l WHERE
 (COALESCE(l->>'alert_rule_id','')='' OR l->>'alert_rule_id'=s.rule_id) AND
 (COALESCE(l->>'alert_instance_id','')='' OR l->>'alert_instance_id'=s.id) AND
 (COALESCE(l->>'alert_fingerprint','')='' OR l->>'alert_fingerprint'=s.fingerprint)))`
	result := make([]CorrelationAlert, 0, CorrelationPageSize)
	next, done, err := s.readCorrelationPage(ctx, "alert_instances", "s.id,s.rule_id,s.fingerprint,s.severity,s.first_fired_at,"+correlationAlertWitnessSQL, predicate, []any{w.From.UTC(), w.Through.UTC(), scope.AssetIDs, scope.GroupID, links}, cursor, func(rows pgx.Rows) (string, error) {
		var row CorrelationAlert
		var payload []byte
		if err := rows.Scan(&row.ID, &row.RuleID, &row.Fingerprint, &row.Severity, &row.FirstFiredAt, &payload); err != nil {
			return "", err
		}
		row.FirstFiredAt = row.FirstFiredAt.UTC()
		if len(payload) > 0 {
			var scope alerts.FiringScope
			decodeErr := json.Unmarshal(payload, &scope)
			normalized, normalizeErr := alerts.NormalizeFiringScope(scope)
			if decodeErr != nil || normalizeErr != nil {
				row = CorrelationAlert{ID: row.ID, Rejected: true}
			} else {
				row.FiringScope = &normalized
			}
		}
		result = append(result, row)
		return row.ID, nil
	})
	if err != nil {
		return nil, cursor, false, err
	}
	return result, next, done, nil
}

func (s *PostgresStore) ReadAuditCorrelationPage(ctx context.Context, w CorrelationWindow, scope IncidentCorrelationScope, cursor CorrelationCursor) ([]CorrelationAudit, CorrelationCursor, bool, error) {
	if err := validateCorrelationQuery(w, scope); err != nil {
		return nil, cursor, false, err
	}
	result := make([]CorrelationAudit, 0, CorrelationPageSize)
	next, done, err := s.readCorrelationPage(ctx, "audit_events", "s.id,s.type,COALESCE(s.actor_id,''),COALESCE(s.target,''),s.timestamp", `s.timestamp >= $1 AND s.timestamp <= $2 AND (s.target=$3 OR s.target=ANY($4::text[]))`, []any{w.From.UTC(), w.Through.UTC(), scope.IncidentID, scope.OperationAssetIDs}, cursor, func(rows pgx.Rows) (string, error) {
		var row CorrelationAudit
		err := rows.Scan(&row.ID, &row.Type, &row.ActorID, &row.Target, &row.Timestamp)
		row.Timestamp = row.Timestamp.UTC()
		result = append(result, row)
		return row.ID, err
	})
	if err != nil {
		return nil, cursor, false, err
	}
	return result, next, done, nil
}
