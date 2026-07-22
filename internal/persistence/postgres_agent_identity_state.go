package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/enrollment"
)

const (
	agentEnrollmentIssuanceLock = "agent-enrollment-token-issuance"
	agentFleetCapacityLock      = "agent-fleet-capacity"
)

// lockAgentEnrollmentIssuance serializes enrollment-token creation with
// identity marker changes. A token issued before a credential rotation can
// therefore never become valid for a later recovery because of transaction
// interleaving or application clock skew.
func lockAgentEnrollmentIssuance(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		agentEnrollmentIssuanceLock,
	)
	return err
}

func lockAgentFleetCapacity(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		agentFleetCapacityLock,
	)
	return err
}

// ensureAgentFleetCapacity counts durable identities plus live approval
// reservations. Token revocation and expiry intentionally do not free a
// durable identity; only explicit asset decommission does so.
func ensureAgentFleetCapacity(ctx context.Context, tx pgx.Tx, configuredLimit int) error {
	if err := lockAgentFleetCapacity(ctx, tx); err != nil {
		return err
	}
	limit := enrollment.BoundedLimit(configuredLimit, enrollment.DefaultMaxEnrolledAgents, enrollment.HardMaxEnrolledAgents)
	var count int
	err := tx.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM (
			SELECT asset_id FROM agent_identity_state
			UNION
			SELECT asset_id
			FROM agent_tokens
			WHERE status = 'pending' AND revoked_at IS NULL AND expires_at > clock_timestamp()
		 ) AS enrolled_or_reserved`,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count >= limit {
		return ErrAgentFleetCapacityReached
	}
	return nil
}

// selectAgentIdentityMarkerForUpdate returns the durable credential-rotation
// marker. The fallback/backfill path protects installations upgraded while an
// enrollment request was already in flight.
func selectAgentIdentityMarkerForUpdate(ctx context.Context, tx pgx.Tx, assetID string) (time.Time, error) {
	assetID = strings.TrimSpace(assetID)
	var marker time.Time
	err := tx.QueryRow(ctx,
		`SELECT credential_rotated_at
		 FROM agent_identity_state
		 WHERE asset_id = $1
		 FOR UPDATE`,
		assetID,
	).Scan(&marker)
	if err == nil {
		return marker.UTC(), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, err
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO agent_identity_state (asset_id, credential_rotated_at)
		 SELECT a.id, GREATEST(a.created_at, COALESCE(MAX(t.created_at), a.created_at))
		 FROM assets a
		 LEFT JOIN agent_tokens t ON t.asset_id = a.id
		 WHERE a.id = $1
		 GROUP BY a.id, a.created_at
		 ON CONFLICT (asset_id) DO UPDATE SET
			credential_rotated_at = GREATEST(agent_identity_state.credential_rotated_at, EXCLUDED.credential_rotated_at)
		 RETURNING credential_rotated_at`,
		assetID,
	).Scan(&marker)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, err
	}
	return marker.UTC(), nil
}

func markAgentIdentityRotated(ctx context.Context, tx pgx.Tx, assetID string) (time.Time, error) {
	var marker time.Time
	err := tx.QueryRow(ctx,
		`INSERT INTO agent_identity_state (asset_id, credential_rotated_at)
		 VALUES ($1, clock_timestamp())
		 ON CONFLICT (asset_id) DO UPDATE SET credential_rotated_at = clock_timestamp()
		 RETURNING credential_rotated_at`,
		strings.TrimSpace(assetID),
	).Scan(&marker)
	return marker.UTC(), err
}

// databaseTTL converts an API absolute expiry into a duration at the
// persistence boundary. PostgreSQL then anchors the actual expiry to its own
// clock, avoiding application/DB clock-skew authorization bugs.
func databaseTTL(expiresAt time.Time) (time.Duration, error) {
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return 0, fmt.Errorf("token expiry must be in the future")
	}
	return ttl, nil
}
