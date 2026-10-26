package persistence

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/incidents"
)

var (
	_ IncidentAssetScopeReader = (*PostgresStore)(nil)
	_ IncidentAlertScopeReader = (*PostgresStore)(nil)
	_ GroupAssetIDScopeReader  = (*PostgresStore)(nil)
)

func (s *PostgresStore) ReadIncidentAssetsContext(ctx context.Context, incidentID string, visit func(incidents.IncidentAsset) error) error {
	return readIncidentScopePages(ctx, s, incidentID, true,
		`SELECT id, incident_id, asset_id, role, created_at FROM incident_assets
		 WHERE incident_id=$1 AND ($2 = '' OR id COLLATE "C" > $2 COLLATE "C")
		 ORDER BY id COLLATE "C" LIMIT $3`,
		func(row pgx.Row) (incidents.IncidentAsset, error) { return scanIncidentAsset(row) },
		func(row incidents.IncidentAsset) string { return row.ID },
		func(row incidents.IncidentAsset) error { return validateIncidentAssetScope(row, incidentID) }, visit)
}

func (s *PostgresStore) ReadIncidentAlertLinksContext(ctx context.Context, incidentID string, visit func(incidents.AlertLink) error) error {
	return readIncidentScopePages(ctx, s, incidentID, true,
		`SELECT id, incident_id, alert_rule_id, alert_instance_id, alert_fingerprint,
		 link_type, created_by, created_at FROM incident_alert_links
		 WHERE incident_id=$1 AND ($2 = '' OR id COLLATE "C" > $2 COLLATE "C")
		 ORDER BY id COLLATE "C" LIMIT $3`,
		func(row pgx.Row) (incidents.AlertLink, error) { return scanIncidentAlertLink(row) },
		func(row incidents.AlertLink) string { return row.ID },
		func(row incidents.AlertLink) error { return validateIncidentAlertScope(row, incidentID) }, visit)
}

func (s *PostgresStore) ReadGroupAssetIDsContext(ctx context.Context, groupID string, visit func(string) error) error {
	return readIncidentScopePages(ctx, s, groupID, false,
		`SELECT id FROM assets WHERE group_id=$1 AND ($2 = '' OR id COLLATE "C" > $2 COLLATE "C")
		 ORDER BY id COLLATE "C" LIMIT $3`,
		func(row pgx.Row) (string, error) { var id string; err := row.Scan(&id); return id, err },
		func(id string) string { return id }, func(string) error { return nil }, visit)
}

// Each scope kind uses the same short snapshot and cursor ownership. No
// transaction escapes this call or stays open across worker slices.
func readIncidentScopePages[T any](ctx context.Context, s *PostgresStore, ownerID string, incident bool,
	query string, scan func(pgx.Row) (T, error), key func(T) string, validate func(T) error, visit func(T) error) error {
	ctx, cancel := context.WithTimeout(ctx, incidentScopeReadTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validIncidentScopeID(ownerID) || visit == nil {
		return fmt.Errorf("invalid incident scope read")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if incident {
		var found bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM incidents WHERE id=$1)`, ownerID).Scan(&found); err != nil {
			return err
		}
		if !found {
			return incidents.ErrIncidentNotFound
		}
	}
	after := ""
	for {
		rows, err := tx.Query(ctx, query, ownerID, after, incidentScopePageSize)
		if err != nil {
			return err
		}
		page := make([]T, 0, incidentScopePageSize)
		for rows.Next() {
			row, scanErr := scan(rows)
			if scanErr != nil {
				rows.Close()
				return scanErr
			}
			page = append(page, row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, row := range page {
			if err := ctx.Err(); err != nil {
				return err
			}
			id := key(row)
			if !validIncidentScopeID(id) || id <= after {
				return fmt.Errorf("invalid incident scope page order")
			}
			if err := validate(row); err != nil {
				return err
			}
			if err := visit(row); err != nil {
				return err
			}
			after = id
		}
		if len(page) < incidentScopePageSize {
			return tx.Commit(ctx)
		}
	}
}
