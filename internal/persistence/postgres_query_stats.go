package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrPGStatStatementsUnavailable indicates pg_stat_statements is not available
// in the current database runtime.
var ErrPGStatStatementsUnavailable = errors.New("pg_stat_statements unavailable")

// QueryStat summarizes a pg_stat_statements row for lightweight operator
// observability in worker/runtime status endpoints.
type QueryStat struct {
	QueryID          string  `json:"query_id"`
	Calls            int64   `json:"calls"`
	TotalExecTimeMS  float64 `json:"total_exec_time_ms"`
	MeanExecTimeMS   float64 `json:"mean_exec_time_ms"`
	Rows             int64   `json:"rows"`
	SharedBlocksHit  int64   `json:"shared_blocks_hit"`
	SharedBlocksRead int64   `json:"shared_blocks_read"`
	TempBlocksWrote  int64   `json:"temp_blocks_wrote"`
	Query            string  `json:"query"`
}

func (s *PostgresStore) TopQueryStats(limit int) ([]QueryStat, error) {
	if limit < 0 {
		limit = 5
	}
	if limit > 5000 {
		limit = 5000
	}

	sql := `SELECT
			queryid::text,
			calls,
			total_exec_time,
			mean_exec_time,
			rows,
			shared_blks_hit,
			shared_blks_read,
			temp_blks_written,
			query
		 FROM pg_stat_statements
		 WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
		 ORDER BY total_exec_time DESC`

	var (
		rows pgx.Rows
		err  error
	)
	if limit == 0 {
		rows, err = s.pool.Query(context.Background(), sql)
	} else {
		sql += " LIMIT $1"
		rows, err = s.pool.Query(context.Background(), sql, limit)
	}
	if err != nil {
		if isPGStatStatementsUnavailable(err) {
			return nil, ErrPGStatStatementsUnavailable
		}
		return nil, err
	}
	defer rows.Close()

	out := make([]QueryStat, 0, limit)
	for rows.Next() {
		entry := QueryStat{}
		if err := rows.Scan(
			&entry.QueryID,
			&entry.Calls,
			&entry.TotalExecTimeMS,
			&entry.MeanExecTimeMS,
			&entry.Rows,
			&entry.SharedBlocksHit,
			&entry.SharedBlocksRead,
			&entry.TempBlocksWrote,
			&entry.Query,
		); err != nil {
			return nil, err
		}
		entry.Query = normalizeQueryPreview(entry.Query)
		out = append(out, entry)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func isPGStatStatementsUnavailable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "42P01", "42501":
			if strings.Contains(strings.ToLower(pgErr.Message), "pg_stat_statements") || pgErr.Code == "42501" {
				return true
			}
		}
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(lower, "pg_stat_statements") &&
		(strings.Contains(lower, "does not exist") || strings.Contains(lower, "permission denied"))
}

func normalizeQueryPreview(query string) string {
	normalized := strings.Join(strings.Fields(query), " ")
	const maxLen = 240
	if len(normalized) <= maxLen {
		return normalized
	}
	if maxLen <= 3 {
		return normalized[:maxLen]
	}
	return normalized[:maxLen-3] + "..."
}
