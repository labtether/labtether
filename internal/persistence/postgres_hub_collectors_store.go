package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/idgen"
)

// --- scan helpers ---

type hubCollectorScanner interface {
	Scan(dest ...any) error
}

func scanHubCollector(row hubCollectorScanner) (hubcollector.Collector, error) {
	c := hubcollector.Collector{}
	var config []byte
	var lastCollectedAt *time.Time
	var lastStatus *string
	var lastError *string
	if err := row.Scan(
		&c.ID,
		&c.AssetID,
		&c.CollectorType,
		&config,
		&c.Enabled,
		&c.IntervalSeconds,
		&lastCollectedAt,
		&lastStatus,
		&lastError,
		&c.CreatedAt,
		&c.UpdatedAt,
	); err != nil {
		return hubcollector.Collector{}, err
	}
	c.Config = unmarshalAnyMap(config)
	if lastCollectedAt != nil {
		value := lastCollectedAt.UTC()
		c.LastCollectedAt = &value
	}
	if lastStatus != nil {
		c.LastStatus = *lastStatus
	}
	if lastError != nil {
		c.LastError = *lastError
	}
	c.CreatedAt = c.CreatedAt.UTC()
	c.UpdatedAt = c.UpdatedAt.UTC()
	return c, nil
}

// --- columns ---

const hubCollectorColumns = `id, asset_id, collector_type, config, enabled, interval_seconds, last_collected_at, last_status, last_error, created_at, updated_at`

// --- store methods ---

func (s *PostgresStore) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	now := time.Now().UTC()

	collectorType := hubcollector.NormalizeCollectorType(req.CollectorType)
	if collectorType == "" {
		return hubcollector.Collector{}, errors.New("invalid collector_type")
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
		return hubcollector.Collector{}, err
	}

	c, err := scanHubCollector(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`INSERT INTO hub_collectors (%s)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6, NULL, '', '', $7, $7)
		 RETURNING %s`, hubCollectorColumns, hubCollectorColumns),
		idgen.New("hc"),
		strings.TrimSpace(req.AssetID),
		collectorType,
		configPayload,
		enabled,
		intervalSeconds,
		now,
	))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") {
			return hubcollector.Collector{}, errors.New("hub collector already exists for this asset")
		}
		return hubcollector.Collector{}, err
	}
	return c, nil
}

func (s *PostgresStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	c, err := scanHubCollector(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT %s FROM hub_collectors WHERE id = $1`, hubCollectorColumns),
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return hubcollector.Collector{}, false, nil
		}
		return hubcollector.Collector{}, false, err
	}
	return c, true, nil
}

func (s *PostgresStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	query := fmt.Sprintf(`SELECT %s FROM hub_collectors`, hubCollectorColumns)
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

	out := make([]hubcollector.Collector, 0, limit)
	for rows.Next() {
		c, scanErr := scanHubCollector(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	existing, ok, err := s.GetHubCollector(id)
	if err != nil {
		return hubcollector.Collector{}, err
	}
	if !ok {
		return hubcollector.Collector{}, hubcollector.ErrCollectorNotFound
	}

	if req.Config != nil {
		existing.Config = cloneAnyMap(*req.Config)
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.IntervalSeconds != nil {
		existing.IntervalSeconds = *req.IntervalSeconds
	}

	configPayload, err := marshalAnyMap(existing.Config)
	if err != nil {
		return hubcollector.Collector{}, err
	}

	now := time.Now().UTC()
	return scanHubCollector(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`UPDATE hub_collectors
		 SET config = $2::jsonb, enabled = $3, interval_seconds = $4, updated_at = $5
		 WHERE id = $1
		 RETURNING %s`, hubCollectorColumns),
		existing.ID,
		configPayload,
		existing.Enabled,
		existing.IntervalSeconds,
		now,
	))
}

func (s *PostgresStore) DeleteHubCollector(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM hub_collectors WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return hubcollector.ErrCollectorNotFound
	}
	return nil
}

func (s *PostgresStore) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE hub_collectors
		 SET last_status = $2, last_error = $3, last_collected_at = $4, updated_at = $5
		 WHERE id = $1`,
		strings.TrimSpace(id),
		strings.TrimSpace(status),
		strings.TrimSpace(lastError),
		collectedAt.UTC(),
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return hubcollector.ErrCollectorNotFound
	}
	return nil
}
