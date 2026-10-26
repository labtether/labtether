package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

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
		          state.retry_count, state.delivery_generation, state.expires_at, state.lease_expires_at
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
		leaseExpiresAt     time.Time
	}
	states := make([]claimedState, 0, limit)
	for rows.Next() {
		var state claimedState
		if err := rows.Scan(&state.deviceID, &state.channelID, &state.windowSeconds,
			&state.retryCount, &state.deliveryGeneration, &state.expiresAt, &state.leaseExpiresAt); err != nil {
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
            WITH due AS (
                SELECT id FROM push_alert_digest_events
                WHERE push_device_id = $1 AND expires_at > $2 AND due_at <= $2
                ORDER BY created_at ASC, id ASC LIMIT $3
            )
            UPDATE push_alert_digest_events event SET delivery_generation = $4
            FROM due WHERE event.id = due.id
            RETURNING event.id, severity, node_offline, service_down, group_ids,
                maintenance_scope_complete, created_at, COALESCE(rule_id, ''), labels,
                retry_count, due_at, expires_at
        `, state.deviceID, now, MaxPushDigestEventsPerDevice, state.deliveryGeneration)
		if err != nil {
			return nil, err
		}
		events := make([]PushDigestEvent, 0, MaxPushDigestEventsPerDevice)
		for eventRows.Next() {
			var event PushDigestEvent
			var groupPayload, labelsPayload []byte
			if err := eventRows.Scan(&event.ID, &event.Severity, &event.NodeOffline,
				&event.ServiceDown, &groupPayload, &event.MaintenanceScopeComplete,
				&event.CreatedAt, &event.RuleID, &labelsPayload, &event.RetryCount, &event.DueAt, &event.ExpiresAt); err != nil {
				eventRows.Close()
				return nil, err
			}
			if len(groupPayload) > 0 {
				if err := json.Unmarshal(groupPayload, &event.GroupIDs); err != nil {
					eventRows.Close()
					return nil, fmt.Errorf("decode push digest group scope: %w", err)
				}
			}
			if len(labelsPayload) > 0 && len(labelsPayload) <= 32768 {
				// Invalid/legacy identity is deferred by the send policy, not guessed.
				if json.Unmarshal(labelsPayload, &event.Labels) != nil {
					event.Labels = nil
				}
			}
			events = append(events, event)
		}
		if err := eventRows.Err(); err != nil {
			eventRows.Close()
			return nil, err
		}
		eventRows.Close()
		sort.Slice(events, func(i, j int) bool {
			if events[i].CreatedAt.Equal(events[j].CreatedAt) {
				return events[i].ID < events[j].ID
			}
			return events[i].CreatedAt.Before(events[j].CreatedAt)
		})

		claims = append(claims, PushDigestClaim{
			Device:             device,
			LeaseExpiresAt:     state.leaseExpiresAt,
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
