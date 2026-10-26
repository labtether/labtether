package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// All SQL fragments here come from the owning typed methods, never callers.
// The short snapshot ends before any caller event upsert or other work.
func (s *PostgresStore) readCorrelationPage(ctx context.Context, table, columns, predicate string, args []any, cursor CorrelationCursor, scan func(pgx.Rows) (string, error)) (CorrelationCursor, bool, error) {
	if err := validateCorrelationCursor(cursor); err != nil {
		return cursor, false, err
	}
	ctx, cancel, err := correlationContext(ctx)
	if err != nil {
		return cursor, false, err
	}
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return cursor, false, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	next := cursor
	if !next.Started {
		if err = tx.QueryRow(ctx, "SELECT COALESCE(MAX(s.id COLLATE \"C\"),'') FROM "+table+" s WHERE "+predicate, args...).Scan(&next.UpperID); err != nil {
			return cursor, false, err
		}
		next.Started = true
	}
	if next.UpperID == "" {
		if err = tx.Commit(ctx); err != nil {
			return cursor, false, err
		}
		return next, true, nil
	}
	n := len(args) + 1
	query := "SELECT " + columns + " FROM " + table + " s WHERE (" + predicate + ")" + fmt.Sprintf(" AND s.id COLLATE \"C\" > $%d COLLATE \"C\" AND s.id COLLATE \"C\" <= $%d COLLATE \"C\" ORDER BY s.id COLLATE \"C\" LIMIT 128", n, n+1)
	params := append(append([]any(nil), args...), cursor.AfterID, next.UpperID)
	rows, err := tx.Query(ctx, query, params...)
	if err != nil {
		return cursor, false, err
	}
	count := 0
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			rows.Close()
			return cursor, false, err
		}
		id, readErr := scan(rows)
		if readErr != nil {
			rows.Close()
			return cursor, false, readErr
		}
		if !correlationExactID(id) || id <= next.AfterID || id > next.UpperID {
			rows.Close()
			return cursor, false, fmt.Errorf("invalid stored correlation cursor identity")
		}
		next.AfterID = id
		count++
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return cursor, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return cursor, false, err
	}
	return next, count < CorrelationPageSize, nil
}
