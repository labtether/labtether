package persistence

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/alerts"
)

func (s *PostgresStore) ReadFiringAlertInstances(ctx context.Context, ruleIDs []string, visit func(alerts.AlertInstance) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	const pageSize = 128
	after := ""
	for {
		rows, err := tx.Query(ctx, `SELECT id, rule_id, fingerprint, status, severity,
		 labels, annotations, started_at, resolved_at, last_fired_at,
		 suppressed_by, created_at, updated_at, first_fired_at, firing_scope
		 FROM alert_instances WHERE status='firing'
		 AND ($1::text[] IS NULL OR rule_id=ANY($1::text[]))
		 AND id COLLATE "C" > $2 COLLATE "C"
		 ORDER BY id COLLATE "C" LIMIT $3`, ruleIDs, after, pageSize)
		if err != nil {
			return err
		}
		page := make([]alerts.AlertInstance, 0, pageSize)
		for rows.Next() {
			row, scanErr := scanAlertInstance(rows)
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
			if err := visit(row); err != nil {
				return err
			}
			after = row.ID
		}
		if len(page) < pageSize {
			return tx.Commit(ctx)
		}
	}
}
