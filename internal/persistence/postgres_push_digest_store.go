package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/idgen"
	"sort"
	"time"
)

// EnqueuePushDigestEvents atomically accepts a bounded batch. The per-device
// state row serializes concurrent routes/hubs, while the unique event key
// prevents the same alert transition from being counted once per APNs route.
func (s *PostgresStore) EnqueuePushDigestEvents(ctx context.Context, raw []PushDigestEnqueue) (PushDigestEnqueueResult, error) {
	var result PushDigestEnqueueResult
	if len(raw) == 0 {
		return result, nil
	}
	if len(raw) > MaxPushDigestEnqueueBatch {
		return result, fmt.Errorf("push digest enqueue batch exceeds %d events", MaxPushDigestEnqueueBatch)
	}

	events := make([]PushDigestEnqueue, 0, len(raw))
	for _, candidate := range raw {
		event, err := normalizePushDigestEnqueue(candidate)
		if err != nil {
			return result, err
		}
		events = append(events, event)
	}
	// Every concurrent hub acquires per-device state locks in the same order,
	// preventing cross-batch deadlocks during fleet-wide alert fanout.
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].DeviceID == events[j].DeviceID {
			return events[i].DedupeKey < events[j].DedupeKey
		}
		return events[i].DeviceID < events[j].DeviceID
	})

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Pipeline state upserts so a fleet fanout takes one database round trip,
	// while ON CONFLICT DO UPDATE still obtains the row locks that serialize
	// the cap/dedupe decisions below.
	var stateBatch pgx.Batch
	for _, event := range events {
		stateBatch.Queue(`
			INSERT INTO push_alert_digest_states (
				push_device_id, channel_id, window_seconds, due_at, expires_at,
				retry_count, delivery_generation, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, 0, 0, $6, $6)
			ON CONFLICT (push_device_id) DO UPDATE
			   SET window_seconds = EXCLUDED.window_seconds,
			       due_at = LEAST(push_alert_digest_states.due_at, EXCLUDED.due_at),
			       expires_at = GREATEST(push_alert_digest_states.expires_at, EXCLUDED.expires_at),
			       updated_at = GREATEST(push_alert_digest_states.updated_at, EXCLUDED.updated_at)
		`, event.DeviceID, event.ChannelID, event.WindowSeconds,
			event.CreatedAt.Add(time.Duration(event.WindowSeconds)*time.Second),
			event.CreatedAt.Add(PushDigestEventTTL), event.CreatedAt)
	}
	if err := execPushDigestBatch(ctx, tx, &stateBatch, len(events)); err != nil {
		return result, err
	}

	// Expired children cannot consume the per-device cap or block a recycled
	// dedupe key. The state locks above keep this cleanup deterministic.
	var cleanupBatch pgx.Batch
	for _, event := range events {
		cleanupBatch.Queue(`
			DELETE FROM push_alert_digest_events
			 WHERE push_device_id = $1 AND expires_at <= $2
		`, event.DeviceID, event.CreatedAt)
	}
	if err := execPushDigestBatch(ctx, tx, &cleanupBatch, len(events)); err != nil {
		return result, err
	}

	type eventStats struct {
		duplicate bool
		count     int
	}
	stats := make([]eventStats, len(events))
	var statsBatch pgx.Batch
	for _, event := range events {
		statsBatch.Queue(`
			SELECT EXISTS (
				SELECT 1 FROM push_alert_digest_events
				 WHERE push_device_id = $1 AND dedupe_key = $2
			), COUNT(*)
			  FROM push_alert_digest_events
			 WHERE push_device_id = $1
		`, event.DeviceID, event.DedupeKey)
	}
	statsResults := tx.SendBatch(ctx, &statsBatch)
	for index := range events {
		if err := statsResults.QueryRow().Scan(&stats[index].duplicate, &stats[index].count); err != nil {
			_ = statsResults.Close()
			return result, err
		}
	}
	if err := statsResults.Close(); err != nil {
		return result, err
	}

	availableByDevice := make(map[string]int)
	queuedKeysByDevice := make(map[string]map[string]struct{})
	droppedDevices := make(map[string]struct{})
	toInsert := make([]PushDigestEnqueue, 0, len(events))
	for index, event := range events {
		if _, initialized := availableByDevice[event.DeviceID]; !initialized {
			availableByDevice[event.DeviceID] = MaxPushDigestEventsPerDevice - stats[index].count
			queuedKeysByDevice[event.DeviceID] = make(map[string]struct{})
		}
		if stats[index].duplicate {
			result.Duplicates++
			continue
		}
		if _, duplicate := queuedKeysByDevice[event.DeviceID][event.DedupeKey]; duplicate {
			result.Duplicates++
			continue
		}
		if availableByDevice[event.DeviceID] <= 0 {
			result.Dropped++
			if _, reported := droppedDevices[event.DeviceID]; !reported {
				result.DroppedDeviceIDs = append(result.DroppedDeviceIDs, event.DeviceID)
				droppedDevices[event.DeviceID] = struct{}{}
			}
			continue
		}
		queuedKeysByDevice[event.DeviceID][event.DedupeKey] = struct{}{}
		availableByDevice[event.DeviceID]--
		toInsert = append(toInsert, event)
	}

	var insertBatch pgx.Batch
	for _, event := range toInsert {
		labels, err := json.Marshal(event.Labels)
		if err != nil {
			return result, fmt.Errorf("encode push digest identity: %w", err)
		}
		groupIDs, err := json.Marshal(event.GroupIDs)
		if err != nil {
			return result, fmt.Errorf("encode push digest group scope: %w", err)
		}
		insertBatch.Queue(`
			INSERT INTO push_alert_digest_events (
				id, push_device_id, dedupe_key, severity, node_offline,
				service_down, group_ids, maintenance_scope_complete,
				expires_at, created_at, rule_id, labels, due_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12::jsonb, $13)
			ON CONFLICT (push_device_id, dedupe_key) DO NOTHING
		`, idgen.New("pdge"), event.DeviceID, event.DedupeKey, event.Severity,
			event.NodeOffline, event.ServiceDown, string(groupIDs), event.MaintenanceScopeComplete,
			event.CreatedAt.Add(PushDigestEventTTL), event.CreatedAt, event.RuleID, string(labels), event.CreatedAt.Add(time.Duration(event.WindowSeconds)*time.Second))
	}
	insertResults := tx.SendBatch(ctx, &insertBatch)
	for range toInsert {
		tag, err := insertResults.Exec()
		if err != nil {
			_ = insertResults.Close()
			return result, err
		}
		if tag.RowsAffected() == 0 {
			result.Duplicates++
		} else {
			result.Inserted++
		}
	}
	if err := insertResults.Close(); err != nil {
		return result, err
	}

	if err := tx.Commit(ctx); err != nil {
		return PushDigestEnqueueResult{}, err
	}
	return result, nil
}

func execPushDigestBatch(ctx context.Context, tx pgx.Tx, batch *pgx.Batch, count int) error {
	if count == 0 {
		return nil
	}
	results := tx.SendBatch(ctx, batch)
	for range count {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return err
		}
	}
	return results.Close()
}
