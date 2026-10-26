package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// SettlePushDigestClaim commits one disjoint partition of exactly the leased
// snapshot. New arrivals and another generation's rows are never retired.
func (s *PostgresStore) SettlePushDigestClaim(ctx context.Context, deviceID string, generation int64, outcome PushDigestSettlement, now time.Time) (bool, error) {
	if strings.TrimSpace(deviceID) == "" || generation <= 0 {
		return false, nil
	}
	now = now.UTC()
	partition, err := pushDigestPartition(outcome, now)
	if err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var held int64
	if err := tx.QueryRow(ctx, `SELECT delivery_generation FROM push_alert_digest_states
 WHERE push_device_id = $1 AND delivery_generation = $2 FOR UPDATE`, deviceID, generation).Scan(&held); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM push_alert_digest_events WHERE push_device_id=$1 AND delivery_generation=$2`, deviceID, generation)
	if err != nil {
		return false, err
	}
	count := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		if _, ok := partition[id]; !ok {
			rows.Close()
			return false, fmt.Errorf("push digest settlement omits held event")
		}
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if count != len(partition) {
		return false, fmt.Errorf("push digest settlement contains unheld event")
	}

	retired := append(append([]string(nil), outcome.Sent...), outcome.Discarded...)
	if len(retired) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM push_alert_digest_events WHERE push_device_id=$1 AND delivery_generation=$2 AND id=ANY($3::text[])`, deviceID, generation, retired); err != nil {
			return false, err
		}
	}
	if len(outcome.Deferred) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE push_alert_digest_events SET due_at=GREATEST(due_at,$4), delivery_generation=0
   WHERE push_device_id=$1 AND delivery_generation=$2 AND id=ANY($3::text[])`, deviceID, generation, outcome.Deferred, outcome.DeferredUntil.UTC()); err != nil {
			return false, err
		}
	}
	var retryBatch pgx.Batch
	for id, next := range outcome.RetryAt {
		retryBatch.Queue(`UPDATE push_alert_digest_events SET due_at=$4, retry_count=LEAST(retry_count+1,8), delivery_generation=0
   WHERE push_device_id=$1 AND delivery_generation=$2 AND id=$3`, deviceID, generation, id, next.UTC())
	}
	if err := execPushDigestBatch(ctx, tx, &retryBatch, len(outcome.RetryAt)); err != nil {
		return false, err
	}
	// Expiry is the only retirement rule applied to rows outside this claim.
	if _, err := tx.Exec(ctx, `DELETE FROM push_alert_digest_events WHERE push_device_id=$1 AND expires_at <= $2`, deviceID, now); err != nil {
		return false, err
	}
	var nextDue *time.Time
	var retries int
	if err := tx.QueryRow(ctx, `SELECT MIN(due_at),COALESCE(MAX(retry_count),0) FROM push_alert_digest_events WHERE push_device_id=$1`, deviceID).Scan(&nextDue, &retries); err != nil {
		return false, err
	}
	if nextDue == nil {
		if _, err := tx.Exec(ctx, `DELETE FROM push_alert_digest_states WHERE push_device_id=$1 AND delivery_generation=$2`, deviceID, generation); err != nil {
			return false, err
		}
	} else {
		// New due arrivals may run on the next worker tick; blocked events retain
		// their own future due time and cannot be pulled forward by those arrivals.
		if _, err := tx.Exec(ctx, `UPDATE push_alert_digest_states SET due_at=$3,retry_count=$4,lease_expires_at=NULL,updated_at=$5
   WHERE push_device_id=$1 AND delivery_generation=$2`, deviceID, generation, nextDue.UTC(), retries, now); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func pushDigestPartition(outcome PushDigestSettlement, now time.Time) (map[string]struct{}, error) {
	ids := make(map[string]struct{})
	add := func(id string) error {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("empty push digest settlement ID")
		}
		if _, exists := ids[id]; exists {
			return fmt.Errorf("overlapping push digest settlement sets")
		}
		ids[id] = struct{}{}
		if len(ids) > MaxPushDigestEventsPerDevice {
			return fmt.Errorf("push digest settlement exceeds bound")
		}
		return nil
	}
	for _, set := range [][]string{outcome.Sent, outcome.Deferred, outcome.Discarded} {
		for _, id := range set {
			if err := add(id); err != nil {
				return nil, err
			}
		}
	}
	if len(outcome.Deferred) > 0 && !outcome.DeferredUntil.After(now) {
		return nil, fmt.Errorf("push digest deferral must be future")
	}
	for id, next := range outcome.RetryAt {
		if err := add(id); err != nil {
			return nil, err
		}
		if !next.After(now) {
			return nil, fmt.Errorf("push digest retry must be future")
		}
	}
	return ids, nil
}
