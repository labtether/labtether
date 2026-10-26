package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const correlationIncidentColumns = "s.id,s.status,COALESCE(s.group_id,''),COALESCE(s.primary_asset_id,''),s.opened_at"

func (s *PostgresStore) ReadActiveIncidentCorrelationPage(ctx context.Context, through time.Time, cursor CorrelationCursor) ([]CorrelationIncident, CorrelationCursor, bool, error) {
	if through.IsZero() {
		return nil, cursor, false, errors.New("incident correlation cutoff required")
	}
	result := make([]CorrelationIncident, 0, CorrelationPageSize)
	next, done, err := s.readCorrelationPage(ctx, "incidents", correlationIncidentColumns, "s.status IN ('open','investigating') AND s.created_at <= $1", []any{through.UTC()}, cursor, func(rows pgx.Rows) (string, error) {
		var row CorrelationIncident
		err := rows.Scan(&row.ID, &row.Status, &row.GroupID, &row.PrimaryAssetID, &row.OpenedAt)
		row.OpenedAt = row.OpenedAt.UTC()
		result = append(result, row)
		return row.ID, err
	})
	if err != nil {
		return nil, cursor, false, err
	}
	return result, next, done, nil
}

func (s *PostgresStore) ReadIncidentCorrelation(ctx context.Context, id string) (CorrelationIncident, bool, error) {
	if !correlationExactID(id) {
		return CorrelationIncident{}, false, errors.New("invalid incident correlation identity")
	}
	ctx, cancel, err := correlationContext(ctx)
	if err != nil {
		return CorrelationIncident{}, false, err
	}
	defer cancel()
	var row CorrelationIncident
	err = s.pool.QueryRow(ctx, "SELECT "+correlationIncidentColumns+" FROM incidents s WHERE s.id=$1", id).Scan(&row.ID, &row.Status, &row.GroupID, &row.PrimaryAssetID, &row.OpenedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CorrelationIncident{}, false, nil
	}
	if err != nil {
		return CorrelationIncident{}, false, err
	}
	row.OpenedAt = row.OpenedAt.UTC()
	return row, true, nil
}

func (s *PostgresStore) ReadActionCorrelationPage(ctx context.Context, w CorrelationWindow, scope IncidentCorrelationScope, cursor CorrelationCursor) ([]CorrelationAction, CorrelationCursor, bool, error) {
	if err := validateCorrelationQuery(w, scope); err != nil {
		return nil, cursor, false, err
	}
	result := make([]CorrelationAction, 0, CorrelationPageSize)
	next, done, err := s.readCorrelationPage(ctx, "action_runs", "s.id,s.type,s.target,s.created_at", `s.status='failed' AND s.created_at >= $1 AND s.created_at <= $2 AND s.target=ANY($3::text[])`, []any{w.From.UTC(), w.Through.UTC(), scope.OperationAssetIDs}, cursor, func(rows pgx.Rows) (string, error) {
		var row CorrelationAction
		err := rows.Scan(&row.ID, &row.Type, &row.Target, &row.CreatedAt)
		row.CreatedAt = row.CreatedAt.UTC()
		result = append(result, row)
		return row.ID, err
	})
	if err != nil {
		return nil, cursor, false, err
	}
	return result, next, done, nil
}

// Use a set predicate and one exact failed target, never one row per result.
const correlationFailedUpdateTarget = `(SELECT MIN((r->>'target') COLLATE "C") FROM jsonb_array_elements(CASE WHEN jsonb_typeof(s.results)='array' THEN s.results ELSE '[]'::jsonb END) r WHERE r->>'status'='failed' AND jsonb_typeof(r->'target')='string' AND r->>'target'=ANY($3::text[]))`

func (s *PostgresStore) ReadUpdateCorrelationPage(ctx context.Context, w CorrelationWindow, scope IncidentCorrelationScope, cursor CorrelationCursor) ([]CorrelationUpdate, CorrelationCursor, bool, error) {
	if err := validateCorrelationQuery(w, scope); err != nil {
		return nil, cursor, false, err
	}
	result := make([]CorrelationUpdate, 0, CorrelationPageSize)
	predicate := `s.status='failed' AND s.created_at >= $1 AND s.created_at <= $2 AND ` + correlationFailedUpdateTarget + ` IS NOT NULL`
	next, done, err := s.readCorrelationPage(ctx, "update_runs", "s.id,s.plan_id,"+correlationFailedUpdateTarget+",s.created_at", predicate, []any{w.From.UTC(), w.Through.UTC(), scope.OperationAssetIDs}, cursor, func(rows pgx.Rows) (string, error) {
		var row CorrelationUpdate
		err := rows.Scan(&row.ID, &row.PlanID, &row.Target, &row.CreatedAt)
		row.CreatedAt = row.CreatedAt.UTC()
		result = append(result, row)
		return row.ID, err
	})
	if err != nil {
		return nil, cursor, false, err
	}
	return result, next, done, nil
}
