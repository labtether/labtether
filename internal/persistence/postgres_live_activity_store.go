package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// Incident Live Activities are intentionally a small, user-initiated set.
	// These hard bounds prevent an authenticated read principal from turning an
	// incident transition into unbounded database or APNs fanout work.
	MaxLiveActivityRegistrationsPerUser     = 16
	MaxLiveActivityRegistrationsPerIncident = 1024
)

var ErrLiveActivityRegistrationLimit = errors.New("live activity registration limit reached")

// LiveActivityPushToken is the server-side routing record for one ActivityKit
// activity. TokenCiphertext is encrypted before it reaches persistence; the
// hash supports ownership-safe deduplication without exposing the APNs token.
type LiveActivityPushToken struct {
	ID                             string
	UserID                         string
	DeviceID                       string
	ActivityID                     string
	IncidentID                     string
	TokenCiphertext                string
	TokenHash                      string
	BundleID                       string
	Environment                    string
	ShowFullDetails                bool
	RetryCount                     int
	DeliveryGeneration             int64
	NextRetryAt                    *time.Time
	PendingStateCiphertext         string
	LastDeliveredIncidentUpdatedAt *time.Time
	ExpiresAt                      time.Time
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

// UpsertLiveActivityPushToken atomically reclaims an ActivityKit token from an
// obsolete owner/activity binding before inserting the current authenticated
// binding. A fresh row ID is used so the ciphertext AAD always matches the row.
func (s *PostgresStore) UpsertLiveActivityPushToken(ctx context.Context, token LiveActivityPushToken) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize quota decisions for this user and incident. A count-only check
	// without these transaction-scoped locks can be exceeded by concurrent token
	// rotations because each statement observes a different database snapshot.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "live-activity-user:"+token.UserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "live-activity-incident:"+token.IncidentID); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		DELETE FROM live_activity_push_tokens
		 WHERE (user_id = $1 AND device_id = $2 AND activity_id = $3)
		    OR (token_hash = $4 AND bundle_id = $5 AND environment = $6)
	`, token.UserID, token.DeviceID, token.ActivityID, token.TokenHash, token.BundleID, token.Environment); err != nil {
		return err
	}

	var userCount, incidentCount int
	if err = tx.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)
			   FROM live_activity_push_tokens
			  WHERE user_id = $1 AND expires_at > NOW()),
			(SELECT COUNT(*)
			   FROM live_activity_push_tokens
			  WHERE incident_id = $2 AND expires_at > NOW())
	`, token.UserID, token.IncidentID).Scan(&userCount, &incidentCount); err != nil {
		return err
	}
	if userCount >= MaxLiveActivityRegistrationsPerUser || incidentCount >= MaxLiveActivityRegistrationsPerIncident {
		return ErrLiveActivityRegistrationLimit
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO live_activity_push_tokens (
			id, user_id, device_id, activity_id, incident_id,
			token_ciphertext, token_hash, bundle_id, environment,
			show_full_details, retry_count, next_retry_at, pending_state_ciphertext, expires_at,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0, NULL, '', $11, NOW(), NOW())
	`,
		token.ID,
		token.UserID,
		token.DeviceID,
		token.ActivityID,
		token.IncidentID,
		token.TokenCiphertext,
		token.TokenHash,
		token.BundleID,
		token.Environment,
		token.ShowFullDetails,
		token.ExpiresAt,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteLiveActivityPushToken removes only the exact activity binding owned by
// the authenticated user and device.
func (s *PostgresStore) DeleteLiveActivityPushToken(
	ctx context.Context,
	userID, deviceID, activityID, incidentID string,
) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM live_activity_push_tokens
		 WHERE user_id = $1
		   AND device_id = $2
		   AND activity_id = $3
		   AND incident_id = $4
	`, userID, deviceID, activityID, incidentID)
	return err
}

// DeleteLiveActivityPushTokenByOwnerAndID is the exact compensating-delete
// path used when an iOS registration request completes after local teardown.
// Including the opaque row ID prevents that stale completion from deleting a
// newer token rotation for the same activity binding.
func (s *PostgresStore) DeleteLiveActivityPushTokenByOwnerAndID(
	ctx context.Context,
	userID, deviceID, activityID, incidentID, registrationID string,
) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM live_activity_push_tokens
		 WHERE id = $1
		   AND user_id = $2
		   AND device_id = $3
		   AND activity_id = $4
		   AND incident_id = $5
	`, registrationID, userID, deviceID, activityID, incidentID)
	return err
}

// DeleteLiveActivityPushTokenByID removes a delivery credential after expiry,
// unrecoverable corruption, or a permanent APNs rejection.
func (s *PostgresStore) DeleteLiveActivityPushTokenByID(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM live_activity_push_tokens WHERE id = $1`, id)
	return err
}

// DeleteLiveActivityPushTokenByGeneration removes a registration only if no
// newer delivery has claimed it since the caller's snapshot.
func (s *PostgresStore) DeleteLiveActivityPushTokenByGeneration(ctx context.Context, id string, generation int64) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM live_activity_push_tokens
			 WHERE id = $1 AND delivery_generation = $2
	`, id, generation)
	return err
}

// ListLiveActivityPushTokensForIncident returns only unexpired registrations.
func (s *PostgresStore) ListLiveActivityPushTokensForIncident(
	ctx context.Context,
	incidentID string,
	now time.Time,
) ([]LiveActivityPushToken, error) {
	rows, err := s.pool.Query(ctx, liveActivityPushTokenSelectSQL+`
		 WHERE incident_id = $1 AND expires_at > $2
		 ORDER BY created_at ASC
		 LIMIT $3
	`, incidentID, now, MaxLiveActivityRegistrationsPerIncident)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLiveActivityPushTokens(rows)
}

// ListDueLiveActivityPushTokens returns bounded retry work and omits expired
// rows. Callers delete expired rows separately before scanning due work.
func (s *PostgresStore) ListDueLiveActivityPushTokens(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]LiveActivityPushToken, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, liveActivityPushTokenSelectSQL+`
		 WHERE expires_at > $1
		   AND next_retry_at IS NOT NULL
		   AND next_retry_at <= $1
		 ORDER BY next_retry_at ASC
		 LIMIT $2
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLiveActivityPushTokens(rows)
}

// ListLiveActivityPushTokensForReconciliation finds committed incident state
// that is newer than the registration's last store update, plus orphaned rows
// left by a hard delete. This is the durable recovery path for a process crash
// between incident commit and in-memory dispatch queue delivery.
func (s *PostgresStore) ListLiveActivityPushTokensForReconciliation(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]LiveActivityPushToken, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, liveActivityPushTokenSelectSQL+`
		 WHERE id IN (
			SELECT token.id
			  FROM live_activity_push_tokens AS token
			  LEFT JOIN incidents AS incident ON incident.id = token.incident_id
			 WHERE token.expires_at > $1
			   AND (token.next_retry_at IS NULL OR token.next_retry_at <= $1)
			   AND (
				incident.id IS NULL
				OR token.last_delivered_incident_updated_at IS NULL
				OR incident.updated_at > token.last_delivered_incident_updated_at
			   )
			 ORDER BY token.updated_at ASC
			 LIMIT $2
		 )
		 ORDER BY updated_at ASC
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLiveActivityPushTokens(rows)
}

// ClaimLiveActivityPushDelivery generation-fences one desired state before any
// external APNs work begins. It also rejects a callback whose incident snapshot
// is older than the currently committed incident, preventing an out-of-order
// dispatcher on another hub replica from regressing ActivityKit state. The lease
// makes a process crash recoverable by the retry scanner; a successful generation
// later clears it.
func (s *PostgresStore) ClaimLiveActivityPushDelivery(
	ctx context.Context,
	id string,
	expectedGeneration int64,
	pendingStateCiphertext string,
	desiredIncidentUpdatedAt time.Time,
	leaseUntil time.Time,
	retryCount int,
) (int64, bool, error) {
	var generation int64
	err := s.pool.QueryRow(ctx, `
		UPDATE live_activity_push_tokens
		   SET delivery_generation = delivery_generation + 1,
		       retry_count = $6,
		       next_retry_at = $5,
		       pending_state_ciphertext = $3,
		       updated_at = NOW()
			 WHERE id = $1
			   AND ($2 < 0 OR delivery_generation = $2)
			   AND NOT EXISTS (
				SELECT 1
				  FROM incidents
				 WHERE incidents.id = live_activity_push_tokens.incident_id
				   AND incidents.updated_at > $4
			   )
			   AND expires_at > NOW()
		 RETURNING delivery_generation
	`, id, expectedGeneration, pendingStateCiphertext, desiredIncidentUpdatedAt, leaseUntil, retryCount).Scan(&generation)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return generation, true, nil
}

// MarkLiveActivityPushRetry schedules a retry only for the generation that
// actually failed. A stale completion cannot overwrite newer desired state.
func (s *PostgresStore) MarkLiveActivityPushRetry(
	ctx context.Context,
	id string,
	generation int64,
	retryCount int,
	nextRetryAt time.Time,
	pendingStateCiphertext string,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE live_activity_push_tokens
		   SET retry_count = $3, next_retry_at = $4, pending_state_ciphertext = $5, updated_at = NOW()
		 WHERE id = $1 AND delivery_generation = $2
	`, id, generation, retryCount, nextRetryAt, pendingStateCiphertext)
	return err
}

// ClearLiveActivityPushRetry clears retry state after a successful update only
// if the delivered generation is still current.
func (s *PostgresStore) ClearLiveActivityPushRetry(
	ctx context.Context,
	id string,
	generation int64,
	deliveredIncidentUpdatedAt time.Time,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE live_activity_push_tokens
		   SET retry_count = 0,
		       next_retry_at = NULL,
		       pending_state_ciphertext = '',
		       last_delivered_incident_updated_at = $3,
		       updated_at = NOW()
		 WHERE id = $1 AND delivery_generation = $2
	`, id, generation, deliveredIncidentUpdatedAt)
	return err
}

// DeleteExpiredLiveActivityPushTokens bounds retained credentials even when an
// app cannot deregister during an offline logout.
func (s *PostgresStore) DeleteExpiredLiveActivityPushTokens(ctx context.Context, now time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM live_activity_push_tokens WHERE expires_at <= $1`, now)
	return err
}

// #nosec G101 -- This is a SQL projection over encrypted credential columns, not a hardcoded credential.
const liveActivityPushTokenSelectSQL = `SELECT
	id, user_id, device_id, activity_id, incident_id,
	token_ciphertext, token_hash, bundle_id, environment,
	show_full_details, retry_count, delivery_generation, next_retry_at, pending_state_ciphertext,
	last_delivered_incident_updated_at, expires_at,
	created_at, updated_at
FROM live_activity_push_tokens`

type liveActivityPushTokenRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanLiveActivityPushTokens(rows liveActivityPushTokenRows) ([]LiveActivityPushToken, error) {
	tokens := make([]LiveActivityPushToken, 0)
	for rows.Next() {
		var token LiveActivityPushToken
		if err := rows.Scan(
			&token.ID,
			&token.UserID,
			&token.DeviceID,
			&token.ActivityID,
			&token.IncidentID,
			&token.TokenCiphertext,
			&token.TokenHash,
			&token.BundleID,
			&token.Environment,
			&token.ShowFullDetails,
			&token.RetryCount,
			&token.DeliveryGeneration,
			&token.NextRetryAt,
			&token.PendingStateCiphertext,
			&token.LastDeliveredIncidentUpdatedAt,
			&token.ExpiresAt,
			&token.CreatedAt,
			&token.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}
