package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/synthetic"
)

// --- scan helpers ---

type syntheticCheckScanner interface {
	Scan(dest ...any) error
}

func scanSyntheticCheck(row syntheticCheckScanner) (synthetic.Check, error) {
	check := synthetic.Check{}
	var config []byte
	var lastRunAt *time.Time
	var lastStatus *string
	var serviceID *string
	if err := row.Scan(
		&check.ID,
		&check.Name,
		&check.CheckType,
		&check.Target,
		&config,
		&check.IntervalSeconds,
		&check.Enabled,
		&lastRunAt,
		&lastStatus,
		&check.CreatedAt,
		&check.UpdatedAt,
		&serviceID,
	); err != nil {
		return synthetic.Check{}, err
	}
	check.Config = unmarshalAnyMap(config)
	if lastRunAt != nil {
		value := lastRunAt.UTC()
		check.LastRunAt = &value
	}
	if lastStatus != nil {
		check.LastStatus = *lastStatus
	}
	if serviceID != nil {
		check.ServiceID = *serviceID
	}
	check.CreatedAt = check.CreatedAt.UTC()
	check.UpdatedAt = check.UpdatedAt.UTC()
	return check, nil
}

type syntheticResultScanner interface {
	Scan(dest ...any) error
}

func scanSyntheticResult(row syntheticResultScanner) (synthetic.Result, error) {
	result := synthetic.Result{}
	var latencyMS *int
	var errMsg *string
	var metadata []byte
	if err := row.Scan(
		&result.ID,
		&result.CheckID,
		&result.Status,
		&latencyMS,
		&errMsg,
		&metadata,
		&result.CheckedAt,
	); err != nil {
		return synthetic.Result{}, err
	}
	result.LatencyMS = latencyMS
	if errMsg != nil {
		result.Error = *errMsg
	}
	result.Metadata = unmarshalAnyMap(metadata)
	result.CheckedAt = result.CheckedAt.UTC()
	return result, nil
}

// --- columns ---

const syntheticCheckColumns = `id, name, check_type, target, config, interval_seconds, enabled, last_run_at, last_status, created_at, updated_at, service_id`
const syntheticResultColumns = `id, check_id, status, latency_ms, error, metadata, checked_at`

// --- store methods ---

func (s *PostgresStore) CreateSyntheticCheck(req synthetic.CreateCheckRequest) (synthetic.Check, error) {
	now := time.Now().UTC()

	checkType := synthetic.NormalizeCheckType(req.CheckType)
	if checkType == "" {
		return synthetic.Check{}, errors.New("invalid check_type")
	}

	intervalSeconds := req.IntervalSeconds
	if intervalSeconds <= 0 {
		intervalSeconds = 60
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	configPayload, err := marshalAnyMap(req.Config)
	if err != nil {
		return synthetic.Check{}, err
	}

	return scanSyntheticCheck(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`INSERT INTO synthetic_checks (%s)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, NULL, '', $8, $8, $9)
		 RETURNING %s`, syntheticCheckColumns, syntheticCheckColumns),
		idgen.New("sc"),
		strings.TrimSpace(req.Name),
		checkType,
		strings.TrimSpace(req.Target),
		configPayload,
		intervalSeconds,
		enabled,
		now,
		nullIfBlank(req.ServiceID),
	))
}

func (s *PostgresStore) GetSyntheticCheck(id string) (synthetic.Check, bool, error) {
	check, err := scanSyntheticCheck(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT %s FROM synthetic_checks WHERE id = $1`, syntheticCheckColumns),
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return synthetic.Check{}, false, nil
		}
		return synthetic.Check{}, false, err
	}
	return check, true, nil
}

func (s *PostgresStore) GetSyntheticCheckByServiceID(ctx context.Context, serviceID string) (*synthetic.Check, error) {
	check, err := scanSyntheticCheck(s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM synthetic_checks WHERE service_id = $1 LIMIT 1`, syntheticCheckColumns),
		strings.TrimSpace(serviceID),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &check, nil
}

func (s *PostgresStore) ListSyntheticChecks(limit int, enabledOnly bool) ([]synthetic.Check, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	query := fmt.Sprintf(`SELECT %s FROM synthetic_checks`, syntheticCheckColumns)
	args := make([]any, 0, 2)
	next := 1

	if enabledOnly {
		query += fmt.Sprintf(" WHERE enabled = $%d", next)
		args = append(args, true)
		next++
	}
	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d", next)
	args = append(args, limit)

	rows, err := s.pool.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]synthetic.Check, 0)
	for rows.Next() {
		check, scanErr := scanSyntheticCheck(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, check)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ListDueSyntheticChecks(ctx context.Context, now time.Time, limit int) ([]synthetic.Check, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM synthetic_checks
		 WHERE enabled = true
		   AND interval_seconds > 0
		   AND (last_run_at IS NULL OR last_run_at + make_interval(secs => interval_seconds) <= $1)
		 ORDER BY COALESCE(last_run_at, to_timestamp(0)) ASC, created_at ASC
		 LIMIT $2`, syntheticCheckColumns),
		now.UTC(),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]synthetic.Check, 0)
	for rows.Next() {
		check, scanErr := scanSyntheticCheck(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, check)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateSyntheticCheck(id string, req synthetic.UpdateCheckRequest) (synthetic.Check, error) {
	existing, ok, err := s.GetSyntheticCheck(id)
	if err != nil {
		return synthetic.Check{}, err
	}
	if !ok {
		return synthetic.Check{}, synthetic.ErrCheckNotFound
	}

	if req.Name != nil {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.Target != nil {
		existing.Target = strings.TrimSpace(*req.Target)
	}
	if req.Config != nil {
		existing.Config = cloneAnyMap(*req.Config)
	}
	if req.IntervalSeconds != nil {
		existing.IntervalSeconds = *req.IntervalSeconds
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	configPayload, err := marshalAnyMap(existing.Config)
	if err != nil {
		return synthetic.Check{}, err
	}

	now := time.Now().UTC()
	return scanSyntheticCheck(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`UPDATE synthetic_checks
		 SET name = $2, target = $3, config = $4::jsonb, interval_seconds = $5, enabled = $6, updated_at = $7, service_id = $8
		 WHERE id = $1
		 RETURNING %s`, syntheticCheckColumns),
		existing.ID,
		existing.Name,
		existing.Target,
		configPayload,
		existing.IntervalSeconds,
		existing.Enabled,
		now,
		nullIfBlank(existing.ServiceID),
	))
}

func (s *PostgresStore) DeleteSyntheticCheck(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM synthetic_checks WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return synthetic.ErrCheckNotFound
	}
	return nil
}

func (s *PostgresStore) RecordSyntheticResult(checkID string, result synthetic.Result) (synthetic.Result, error) {
	checkID = strings.TrimSpace(checkID)
	now := time.Now().UTC()

	checkedAt := result.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = now
	}

	metadataPayload, err := marshalAnyMap(result.Metadata)
	if err != nil {
		return synthetic.Result{}, err
	}

	var nullLatency any
	if result.LatencyMS != nil {
		nullLatency = *result.LatencyMS
	}

	recorded, err := scanSyntheticResult(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`INSERT INTO synthetic_check_results (%s)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
		 RETURNING %s`, syntheticResultColumns, syntheticResultColumns),
		idgen.New("scr"),
		checkID,
		strings.TrimSpace(result.Status),
		nullLatency,
		nullIfBlank(result.Error),
		metadataPayload,
		checkedAt.UTC(),
	))
	if err != nil {
		return synthetic.Result{}, err
	}

	// Also update the check's last_run_at / last_status.
	_ = s.UpdateSyntheticCheckStatus(checkID, result.Status, checkedAt)

	return recorded, nil
}

func (s *PostgresStore) ListSyntheticResults(checkID string, limit int) ([]synthetic.Result, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		fmt.Sprintf(`SELECT %s FROM synthetic_check_results
		 WHERE check_id = $1 ORDER BY checked_at DESC LIMIT $2`, syntheticResultColumns),
		strings.TrimSpace(checkID),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]synthetic.Result, 0)
	for rows.Next() {
		r, scanErr := scanSyntheticResult(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateSyntheticCheckStatus(id string, status string, runAt time.Time) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE synthetic_checks
		 SET last_status = $2, last_run_at = $3, updated_at = $4
		 WHERE id = $1`,
		strings.TrimSpace(id),
		strings.TrimSpace(status),
		runAt.UTC(),
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return synthetic.ErrCheckNotFound
	}
	return nil
}
