package persistence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/model"
	"strings"
	"time"
)

func (s *PostgresStore) UpsertIngestCheckpoint(checkpoint model.IngestCheckpoint) (model.IngestCheckpoint, error) {
	providerID := strings.TrimSpace(checkpoint.ProviderInstanceID)
	stream := strings.ToLower(strings.TrimSpace(checkpoint.Stream))
	if providerID == "" || stream == "" {
		return model.IngestCheckpoint{}, ErrNotFound
	}
	syncedAt := checkpoint.SyncedAt.UTC()
	if syncedAt.IsZero() {
		syncedAt = time.Now().UTC()
	}

	stored, err := scanIngestCheckpoint(s.pool.QueryRow(
		context.Background(),
		`INSERT INTO canonical_ingest_checkpoints (
			provider_instance_id, stream, cursor, synced_at
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (provider_instance_id, stream) DO UPDATE SET
			cursor = EXCLUDED.cursor,
			synced_at = EXCLUDED.synced_at
		RETURNING provider_instance_id, stream, cursor, synced_at`,
		providerID,
		stream,
		nullIfBlank(checkpoint.Cursor),
		syncedAt,
	))
	if err != nil {
		return model.IngestCheckpoint{}, err
	}
	return stored, nil
}

func (s *PostgresStore) GetIngestCheckpoint(providerInstanceID, stream string) (model.IngestCheckpoint, bool, error) {
	checkpoint, err := scanIngestCheckpoint(s.pool.QueryRow(
		context.Background(),
		`SELECT provider_instance_id, stream, cursor, synced_at
		 FROM canonical_ingest_checkpoints
		 WHERE provider_instance_id = $1 AND stream = $2`,
		strings.TrimSpace(providerInstanceID),
		strings.ToLower(strings.TrimSpace(stream)),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.IngestCheckpoint{}, false, nil
		}
		return model.IngestCheckpoint{}, false, err
	}
	return checkpoint, true, nil
}

func (s *PostgresStore) RecordReconciliationResult(result model.ReconciliationResult) (model.ReconciliationResult, error) {
	providerID := strings.TrimSpace(result.ProviderInstanceID)
	if providerID == "" {
		return model.ReconciliationResult{}, ErrNotFound
	}
	startedAt := result.StartedAt.UTC()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	finishedAt := result.FinishedAt.UTC()
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}

	stored, _, err := scanReconciliationResult(s.pool.QueryRow(
		context.Background(),
		`INSERT INTO canonical_reconciliation_results (
			id, provider_instance_id, created_count, updated_count,
			stale_count, error_count, started_at, finished_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, provider_instance_id, created_count, updated_count,
			stale_count, error_count, started_at, finished_at`,
		idgen.New("reconcile"),
		providerID,
		result.CreatedCount,
		result.UpdatedCount,
		result.StaleCount,
		result.ErrorCount,
		startedAt,
		finishedAt,
	))
	if err != nil {
		return model.ReconciliationResult{}, err
	}
	return stored, nil
}

func (s *PostgresStore) ListReconciliationResults(providerInstanceID string, limit int) ([]model.ReconciliationResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 5000 {
		limit = 5000
	}
	providerInstanceID = strings.TrimSpace(providerInstanceID)

	var (
		rows pgx.Rows
		err  error
	)
	if providerInstanceID == "" {
		rows, err = s.pool.Query(
			context.Background(),
			`SELECT id, provider_instance_id, created_count, updated_count,
				stale_count, error_count, started_at, finished_at
			 FROM canonical_reconciliation_results
			 ORDER BY finished_at DESC
			 LIMIT $1`,
			limit,
		)
	} else {
		rows, err = s.pool.Query(
			context.Background(),
			`SELECT id, provider_instance_id, created_count, updated_count,
				stale_count, error_count, started_at, finished_at
			 FROM canonical_reconciliation_results
			 WHERE provider_instance_id = $1
			 ORDER BY finished_at DESC
			 LIMIT $2`,
			providerInstanceID,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.ReconciliationResult, 0)
	for rows.Next() {
		result, _, scanErr := scanReconciliationResult(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, result)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) CanonicalStatusWatermark() (time.Time, error) {
	var watermark time.Time
	if err := s.pool.QueryRow(
		context.Background(),
		`SELECT GREATEST(
			COALESCE((SELECT MAX(updated_at) FROM provider_instances), to_timestamp(0)),
			COALESCE((SELECT MAX(updated_at) FROM canonical_capability_sets), to_timestamp(0)),
			COALESCE((SELECT MAX(updated_at) FROM canonical_template_bindings), to_timestamp(0)),
			COALESCE((SELECT MAX(finished_at) FROM canonical_reconciliation_results), to_timestamp(0)),
			COALESCE((SELECT MAX(updated_at) FROM resource_external_refs), to_timestamp(0)),
			COALESCE((SELECT MAX(updated_at) FROM canonical_resource_relationships), to_timestamp(0)),
			COALESCE((SELECT MAX(synced_at) FROM canonical_ingest_checkpoints), to_timestamp(0))
		)`,
	).Scan(&watermark); err != nil {
		return time.Time{}, err
	}
	return watermark.UTC(), nil
}
