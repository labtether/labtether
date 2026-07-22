package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
)

const (
	// MaxPushDigestEventsPerDevice bounds durable work even if an alert storm
	// targets a device that remains offline for the full digest TTL.
	MaxPushDigestEventsPerDevice = 128
	MaxPushDigestGroupIDs        = 64
	MaxPushDigestEnqueueBatch    = 5_000
	MaxPushDigestClaimBatch      = 100
	MaxPushDigestCleanupBatch    = 500
	// PushDigestEventTTL must remain comfortably longer than the maximum
	// 24-hour digest window. Matching the window exactly makes a 24-hour digest
	// expire in ClaimDuePushDigests before it can ever become deliverable, and
	// leaves near-maximum windows no time for the 30-second worker cadence or a
	// bounded retry.
	PushDigestEventTTL = 48 * time.Hour
)

// PushDigestEnqueue is the privacy-minimised alert snapshot retained for a
// server-side iOS digest. It deliberately excludes alert titles, descriptions,
// target names, and APNs tokens.
type PushDigestEnqueue struct {
	DeviceID                 string
	ChannelID                string
	DedupeKey                string
	Severity                 string
	NodeOffline              bool
	ServiceDown              bool
	GroupIDs                 []string
	MaintenanceScopeComplete bool
	WindowSeconds            int
	CreatedAt                time.Time
}

type PushDigestEnqueueResult struct {
	Inserted         int
	Duplicates       int
	Dropped          int
	DroppedDeviceIDs []string
}

// PushDigestEvent is a bounded event snapshot returned only to the delivery
// worker. IDs are opaque and are used to delete exactly the claimed snapshot.
type PushDigestEvent struct {
	ID                       string
	Severity                 string
	NodeOffline              bool
	ServiceDown              bool
	GroupIDs                 []string
	MaintenanceScopeComplete bool
	CreatedAt                time.Time
}

// PushDigestClaim fences one device's due digest across multiple hub
// processes. PushDevice contains the current registration/preferences and must
// be revalidated before APNs delivery.
type PushDigestClaim struct {
	Device             PushDevice
	ChannelID          string
	WindowSeconds      int
	RetryCount         int
	DeliveryGeneration int64
	ExpiresAt          time.Time
	Events             []PushDigestEvent
}

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
		groupIDs, err := json.Marshal(event.GroupIDs)
		if err != nil {
			return result, fmt.Errorf("encode push digest group scope: %w", err)
		}
		insertBatch.Queue(`
			INSERT INTO push_alert_digest_events (
				id, push_device_id, dedupe_key, severity, node_offline,
				service_down, group_ids, maintenance_scope_complete,
				expires_at, created_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)
			ON CONFLICT (push_device_id, dedupe_key) DO NOTHING
		`, idgen.New("pdge"), event.DeviceID, event.DedupeKey, event.Severity,
			event.NodeOffline, event.ServiceDown, string(groupIDs), event.MaintenanceScopeComplete,
			event.CreatedAt.Add(PushDigestEventTTL), event.CreatedAt)
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

// ClaimDuePushDigests leases due per-device batches with SKIP LOCKED. The
// delivery generation fences late completions after a lease expires.
func (s *PostgresStore) ClaimDuePushDigests(ctx context.Context, now time.Time, leaseDuration time.Duration, limit int) ([]PushDigestClaim, error) {
	if leaseDuration < 20*time.Second {
		leaseDuration = 20 * time.Second
	}
	if leaseDuration > 5*time.Minute {
		leaseDuration = 5 * time.Minute
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > MaxPushDigestClaimBatch {
		limit = MaxPushDigestClaimBatch
	}
	now = now.UTC()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		WITH stale AS (
			SELECT state.push_device_id
			  FROM push_alert_digest_states state
			 WHERE state.expires_at <= $1
			    OR NOT EXISTS (
				SELECT 1 FROM push_alert_digest_events event
				 WHERE event.push_device_id = state.push_device_id
				   AND event.expires_at > $1
			    )
			 ORDER BY state.push_device_id ASC
			 LIMIT $2
			 FOR UPDATE OF state SKIP LOCKED
		)
		DELETE FROM push_alert_digest_states state
		 USING stale
		 WHERE state.push_device_id = stale.push_device_id
	`, now, MaxPushDigestCleanupBatch); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		WITH candidates AS (
			SELECT state.push_device_id
			  FROM push_alert_digest_states state
			 WHERE state.due_at <= $1
			   AND state.expires_at > $1
			   AND (state.lease_expires_at IS NULL OR state.lease_expires_at <= $1)
			 ORDER BY state.due_at ASC, state.push_device_id ASC
			 FOR UPDATE OF state SKIP LOCKED
			 LIMIT $2
		)
		UPDATE push_alert_digest_states state
		   SET delivery_generation = state.delivery_generation + 1,
		       lease_expires_at = $3,
		       updated_at = $1
		  FROM candidates
		 WHERE state.push_device_id = candidates.push_device_id
		RETURNING state.push_device_id, state.channel_id, state.window_seconds,
		          state.retry_count, state.delivery_generation, state.expires_at
	`, now, limit, now.Add(leaseDuration))
	if err != nil {
		return nil, err
	}
	type claimedState struct {
		deviceID           string
		channelID          string
		windowSeconds      int
		retryCount         int
		deliveryGeneration int64
		expiresAt          time.Time
	}
	states := make([]claimedState, 0, limit)
	for rows.Next() {
		var state claimedState
		if err := rows.Scan(&state.deviceID, &state.channelID, &state.windowSeconds,
			&state.retryCount, &state.deliveryGeneration, &state.expiresAt); err != nil {
			rows.Close()
			return nil, err
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	claims := make([]PushDigestClaim, 0, len(states))
	for _, state := range states {
		var device PushDevice
		if err := scanPushDevice(tx.QueryRow(ctx, pushDeviceSelectSQL+` WHERE id = $1`, state.deviceID), &device); err != nil {
			return nil, err
		}

		eventRows, err := tx.Query(ctx, `
			SELECT id, severity, node_offline, service_down, group_ids,
			       maintenance_scope_complete, created_at
			  FROM push_alert_digest_events
			 WHERE push_device_id = $1 AND expires_at > $2
			 ORDER BY created_at ASC, id ASC
			 LIMIT $3
		`, state.deviceID, now, MaxPushDigestEventsPerDevice)
		if err != nil {
			return nil, err
		}
		events := make([]PushDigestEvent, 0, MaxPushDigestEventsPerDevice)
		for eventRows.Next() {
			var event PushDigestEvent
			var groupPayload []byte
			if err := eventRows.Scan(&event.ID, &event.Severity, &event.NodeOffline,
				&event.ServiceDown, &groupPayload, &event.MaintenanceScopeComplete,
				&event.CreatedAt); err != nil {
				eventRows.Close()
				return nil, err
			}
			if len(groupPayload) > 0 {
				if err := json.Unmarshal(groupPayload, &event.GroupIDs); err != nil {
					eventRows.Close()
					return nil, fmt.Errorf("decode push digest group scope: %w", err)
				}
			}
			events = append(events, event)
		}
		if err := eventRows.Err(); err != nil {
			eventRows.Close()
			return nil, err
		}
		eventRows.Close()

		claims = append(claims, PushDigestClaim{
			Device:             device,
			ChannelID:          state.channelID,
			WindowSeconds:      state.windowSeconds,
			RetryCount:         state.retryCount,
			DeliveryGeneration: state.deliveryGeneration,
			ExpiresAt:          state.expiresAt,
			Events:             events,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return claims, nil
}

// CompletePushDigestClaim deletes only the event snapshot delivered (or
// intentionally discarded after a preference recheck). Events enqueued while
// APNs was in flight remain scheduled for a fresh window.
func (s *PostgresStore) CompletePushDigestClaim(ctx context.Context, deviceID string, generation int64, eventIDs []string, now time.Time) (bool, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || generation <= 0 || len(eventIDs) == 0 {
		return false, nil
	}
	now = now.UTC()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var windowSeconds int
	if err := tx.QueryRow(ctx, `
		SELECT window_seconds
		  FROM push_alert_digest_states
		 WHERE push_device_id = $1 AND delivery_generation = $2
		 FOR UPDATE
	`, deviceID, generation).Scan(&windowSeconds); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM push_alert_digest_events
		 WHERE push_device_id = $1
		   AND (id = ANY($2::text[]) OR expires_at <= $3)
	`, deviceID, eventIDs, now); err != nil {
		return false, err
	}

	var nextCreatedAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT MIN(created_at)
		  FROM push_alert_digest_events
		 WHERE push_device_id = $1 AND expires_at > $2
	`, deviceID, now).Scan(&nextCreatedAt); err != nil {
		return false, err
	}
	if nextCreatedAt == nil {
		if _, deleteErr := tx.Exec(ctx, `DELETE FROM push_alert_digest_states WHERE push_device_id = $1`, deviceID); deleteErr != nil {
			return false, deleteErr
		}
	} else {
		dueAt := nextCreatedAt.Add(time.Duration(windowSeconds) * time.Second)
		if dueAt.Before(now) {
			dueAt = now
		}
		if _, err := tx.Exec(ctx, `
			UPDATE push_alert_digest_states
			   SET due_at = $3, retry_count = 0, lease_expires_at = NULL, updated_at = $4
			 WHERE push_device_id = $1 AND delivery_generation = $2
		`, deviceID, generation, dueAt, now); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// ReleasePushDigestClaim clears the lease and reschedules without changing the
// event snapshot. Preference/maintenance deferrals do not consume the retry
// budget; delivery failures do.
func (s *PostgresStore) ReleasePushDigestClaim(ctx context.Context, deviceID string, generation int64, nextAttempt time.Time, incrementRetry bool) (bool, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || generation <= 0 {
		return false, nil
	}
	retryIncrement := 0
	if incrementRetry {
		retryIncrement = 1
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE push_alert_digest_states
		   SET due_at = $3,
		       retry_count = LEAST(retry_count + $4, 8),
		       lease_expires_at = NULL,
		       updated_at = $5
		 WHERE push_device_id = $1 AND delivery_generation = $2
	`, deviceID, generation, nextAttempt.UTC(), retryIncrement, time.Now().UTC())
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func normalizePushDigestEnqueue(event PushDigestEnqueue) (PushDigestEnqueue, error) {
	event.DeviceID = strings.TrimSpace(event.DeviceID)
	event.ChannelID = strings.TrimSpace(event.ChannelID)
	event.DedupeKey = strings.ToLower(strings.TrimSpace(event.DedupeKey))
	event.Severity = strings.ToLower(strings.TrimSpace(event.Severity))
	if event.DeviceID == "" || event.ChannelID == "" {
		return PushDigestEnqueue{}, fmt.Errorf("push digest device and channel are required")
	}
	if len(event.DedupeKey) != 64 || !isLowerHex(event.DedupeKey) {
		return PushDigestEnqueue{}, fmt.Errorf("push digest dedupe key must be a 64-character lowercase hex digest")
	}
	if event.Severity != "info" && event.Severity != "warning" {
		return PushDigestEnqueue{}, fmt.Errorf("push digest severity must be info or warning")
	}
	if event.WindowSeconds < 30 {
		event.WindowSeconds = 30
	}
	if event.WindowSeconds > 86_400 {
		event.WindowSeconds = 86_400
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	} else {
		event.CreatedAt = event.CreatedAt.UTC()
	}

	groups := make(map[string]struct{}, len(event.GroupIDs))
	for _, rawGroupID := range event.GroupIDs {
		groupID := strings.TrimSpace(rawGroupID)
		if groupID == "" || len(groupID) > 255 {
			continue
		}
		groups[groupID] = struct{}{}
	}
	event.GroupIDs = make([]string, 0, len(groups))
	for groupID := range groups {
		event.GroupIDs = append(event.GroupIDs, groupID)
	}
	sort.Strings(event.GroupIDs)
	if len(event.GroupIDs) > MaxPushDigestGroupIDs {
		event.GroupIDs = nil
		event.MaintenanceScopeComplete = false
	}
	return event, nil
}

func isLowerHex(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return value != ""
}
