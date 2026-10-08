package persistence

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/enrollment"
	"sync"
	"testing"
	"time"
)

func TestPostgresDecommissionSerializesAuthenticatedHeartbeat(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "agent-decommission-race-" + suffix
	enrollmentHash := "agent-decommission-enrollment-" + suffix
	agentHash := "agent-decommission-token-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_hash = $1`, enrollmentHash)
	})
	now := time.Now().UTC()
	_, _ = store.CreateEnrollmentToken(testEnrollmentTokenParams(enrollmentHash, "race", now.Add(time.Hour), 1))
	result, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: enrollmentHash,
		AgentTokenHash: agentHash, AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	heartbeatErr := make(chan error, 1)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := store.CommitAuthenticatedAgentHeartbeat(ctx, result.AgentToken.ID, assets.HeartbeatRequest{
			AssetID: assetID, Type: "node", Name: assetID, Source: "agent", Status: "online",
		})
		heartbeatErr <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		if err := store.DecommissionAgentAsset(ctx, assetID); err != nil {
			t.Errorf("decommission: %v", err)
		}
	}()
	close(start)
	wg.Wait()
	if err := <-heartbeatErr; err != nil && !errors.Is(err, ErrAgentCredentialInactive) {
		t.Fatalf("heartbeat error=%v", err)
	}
	if _, exists, err := store.GetAsset(assetID); err != nil || exists {
		t.Fatalf("decommissioned PG asset exists=%v err=%v", exists, err)
	}
	if _, valid, err := store.ValidateAgentToken(agentHash); err != nil || valid {
		t.Fatalf("decommissioned PG bearer valid=%v err=%v", valid, err)
	}
}

func TestPostgresDecommissionPermanentlyRetiresAgentIdentity(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "retired-agent-" + suffix
	replacementID := "replacement-agent-" + suffix
	initialHash := "retired-initial-" + suffix
	retryHash := "retired-retry-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = ANY($1)`, []string{assetID, replacementID})
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = ANY($1)`, []string{assetID, replacementID})
		_, _ = store.pool.Exec(ctx, `DELETE FROM retired_agent_identities WHERE asset_id = ANY($1)`, []string{assetID, replacementID})
		_, _ = store.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_hash = ANY($1)`, []string{initialHash, retryHash})
	})

	now := time.Now().UTC()
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(initialHash, "initial", now.Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: initialHash,
		AgentTokenHash: "retired-agent-token-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DecommissionAgentAsset(ctx, assetID); err != nil {
		t.Fatal(err)
	}
	if err := store.DecommissionAgentAsset(ctx, assetID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second decommission error=%v", err)
	}
	var tombstones int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM retired_agent_identities WHERE asset_id = $1`, assetID).Scan(&tombstones); err != nil || tombstones != 1 {
		t.Fatalf("retired identity tombstones=%d err=%v", tombstones, err)
	}
	if retired, err := store.IsAgentIdentityRetired(ctx, assetID); err != nil || !retired {
		t.Fatalf("retired identity lookup=%v err=%v", retired, err)
	}
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Name: "laundered", Type: "host", Source: "manual", Status: "online",
	}); !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("generic heartbeat reused retired identity: %v", err)
	}
	if _, err := store.CommitExistingOwnerAgentHeartbeat(ctx, assets.HeartbeatRequest{
		AssetID: assetID, Name: "legacy", Type: "node", Source: "agent", Status: "online",
	}); !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("legacy owner heartbeat reused retired identity: %v", err)
	}

	retryToken, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(retryHash, "retry", now.Add(time.Hour), 2))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: retryHash,
		AgentTokenHash: "retired-retry-agent-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("retired identity enrollment error=%v", err)
	}
	var useCount int
	if err := store.pool.QueryRow(ctx, `SELECT use_count FROM enrollment_tokens WHERE id = $1`, retryToken.ID).Scan(&useCount); err != nil || useCount != 0 {
		t.Fatalf("retirement rejection token use_count=%d err=%v", useCount, err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: "invalid-token-" + suffix,
		AgentTokenHash: "invalid-agent-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour),
	}); !errors.Is(err, ErrEnrollmentTokenInvalid) {
		t.Fatalf("invalid token leaked retirement state: %v", err)
	}
	if _, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: assetID, AgentTokenHash: "retired-approval-" + suffix, PreparedTokenExpiresAt: now.Add(time.Minute),
	}); !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("retired identity approval error=%v", err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: replacementID, Hostname: replacementID, EnrollmentTokenHash: retryHash,
		AgentTokenHash: "replacement-agent-token-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("new replacement identity enrollment: %v", err)
	}
}

func TestPostgresPreparedApprovalCannotFinalizeRetiredIdentity(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "prepared-retired-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM retired_agent_identities WHERE asset_id = $1`, assetID)
	})
	prepared, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: assetID, AgentTokenHash: "prepared-retired-token-" + suffix,
		PreparedTokenExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO retired_agent_identities (asset_id, retired_at) VALUES ($1, clock_timestamp())`, assetID,
	); err != nil {
		t.Fatal(err)
	}
	_, err = store.FinalizeAgentApproval(ctx, AgentApprovalFinalizeRequest{
		PreparedTokenID: prepared.ID, AssetID: assetID, Hostname: assetID,
		DeviceFingerprint: "LT-RETIRED", DeviceKeyAlgorithm: "ed25519",
		AgentTokenExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("retired prepared approval error=%v", err)
	}
}

func TestPostgresNonAgentDeletionDoesNotRetireIdentity(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "manual-reusable-" + suffix
	enrollmentHash := "manual-reuse-enrollment-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM retired_agent_identities WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_hash = $1`, enrollmentHash)
	})
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Name: "manual", Type: "host", Source: "manual", Status: "online",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DecommissionAgentAsset(ctx, assetID); err != nil {
		t.Fatal(err)
	}
	var tombstones int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM retired_agent_identities WHERE asset_id = $1`, assetID).Scan(&tombstones); err != nil || tombstones != 0 {
		t.Fatalf("non-agent tombstones=%d err=%v", tombstones, err)
	}
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(enrollmentHash, "reuse", time.Now().UTC().Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: enrollmentHash,
		AgentTokenHash: "manual-reuse-agent-" + suffix, AgentTokenExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("non-agent ID was not reusable: %v", err)
	}
}

func TestPostgresAgentSourceCannotBeDowngradedBeforeDecommission(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	assetID := fmt.Sprintf("sticky-agent-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM retired_agent_identities WHERE asset_id = $1`, assetID)
	})
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Name: "legacy agent", Type: "node", Source: "agent", Status: "online",
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Name: "laundered", Type: "host", Source: "manual", Status: "online",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Source != "agent" {
		t.Fatalf("generic heartbeat downgraded agent source to %q", updated.Source)
	}
	if err := store.DecommissionAgentAsset(ctx, assetID); err != nil {
		t.Fatal(err)
	}
	if retired, err := store.IsAgentIdentityRetired(ctx, assetID); err != nil || !retired {
		t.Fatalf("downgrade attempt escaped retirement=%v err=%v", retired, err)
	}
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Name: "reused", Type: "host", Source: "manual", Status: "online",
	}); !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("downgraded agent identity was reusable: %v", err)
	}
}

func TestPostgresCancelledApprovalDoesNotRetireLaterNonAgentAsset(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "cancelled-manual-" + suffix
	pendingHash := "cancelled-manual-pending-" + suffix
	enrollmentHash := "cancelled-manual-enrollment-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM retired_agent_identities WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_hash = $1`, enrollmentHash)
	})
	prepared, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: assetID, AgentTokenHash: pendingHash,
		PreparedTokenExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CancelAgentApproval(ctx, prepared.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Name: "manual", Type: "host", Source: "manual", Status: "online",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DecommissionAgentAsset(ctx, assetID); err != nil {
		t.Fatal(err)
	}
	if retired, err := store.IsAgentIdentityRetired(ctx, assetID); err != nil || retired {
		t.Fatalf("cancelled approval retired later manual asset=%v err=%v", retired, err)
	}
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(enrollmentHash, "reuse", time.Now().UTC().Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: enrollmentHash,
		AgentTokenHash: "cancelled-manual-agent-" + suffix, AgentTokenExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("cancelled approval blocked later enrollment: %v", err)
	}
}

func TestPostgresExistingOwnerHeartbeatCannotRaceDecommission(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "owner-heartbeat-race-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM retired_agent_identities WHERE asset_id = $1`, assetID)
	})
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Type: "node", Name: assetID, Source: "agent",
	}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	heartbeatErr := make(chan error, 1)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := store.CommitExistingOwnerAgentHeartbeat(ctx, assets.HeartbeatRequest{
			AssetID: assetID, Type: "node", Name: assetID, Source: "agent",
		})
		heartbeatErr <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		if err := store.DecommissionAgentAsset(ctx, assetID); err != nil {
			t.Errorf("PG owner race decommission: %v", err)
		}
	}()
	close(start)
	wg.Wait()
	if err := <-heartbeatErr; err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("PG owner heartbeat error=%v", err)
	}
	if _, exists, err := store.GetAsset(assetID); err != nil || exists {
		t.Fatalf("PG owner heartbeat resurrected asset exists=%v err=%v", exists, err)
	}
}

func TestPostgresFleetCapacityPersistsAcrossTokenRevocationUntilDecommission(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	firstAssetID := "capacity-first-" + suffix
	secondAssetID := "capacity-second-" + suffix
	firstEnrollmentHash := "capacity-first-enrollment-" + suffix
	secondEnrollmentHash := "capacity-second-enrollment-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = ANY($1)`, []string{firstAssetID, secondAssetID})
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = ANY($1)`, []string{firstAssetID, secondAssetID})
		_, _ = store.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_hash = ANY($1)`, []string{firstEnrollmentHash, secondEnrollmentHash})
	})
	now := time.Now().UTC()
	var baseline int
	if err := store.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM (
			SELECT asset_id FROM agent_identity_state
			UNION
			SELECT asset_id FROM agent_tokens WHERE status = 'pending' AND revoked_at IS NULL AND expires_at > clock_timestamp()
		) AS enrolled_or_reserved`,
	).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline >= enrollment.HardMaxEnrolledAgents {
		t.Skip("test database is already at the absolute fleet ceiling")
	}
	capacityLimit := baseline + 1
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(firstEnrollmentHash, "first", now.Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: firstAssetID, Hostname: firstAssetID, EnrollmentTokenHash: firstEnrollmentHash,
		AgentTokenHash: "capacity-first-agent-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour), MaxEnrolledAgents: capacityLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeAgentToken(first.AgentToken.ID); err != nil {
		t.Fatal(err)
	}
	secondEnrollment, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(secondEnrollmentHash, "second", now.Add(time.Hour), 1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: secondAssetID, Hostname: secondAssetID, EnrollmentTokenHash: secondEnrollmentHash,
		AgentTokenHash: "capacity-second-agent-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour), MaxEnrolledAgents: capacityLimit,
	})
	if !errors.Is(err, ErrAgentFleetCapacityReached) {
		t.Fatalf("capacity error=%v", err)
	}
	var useCount int
	if err := store.pool.QueryRow(ctx, `SELECT use_count FROM enrollment_tokens WHERE id = $1`, secondEnrollment.ID).Scan(&useCount); err != nil || useCount != 0 {
		t.Fatalf("capacity token use_count=%d err=%v", useCount, err)
	}
	if err := store.DecommissionAgentAsset(ctx, firstAssetID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: secondAssetID, Hostname: secondAssetID, EnrollmentTokenHash: secondEnrollmentHash,
		AgentTokenHash: "capacity-second-agent-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour), MaxEnrolledAgents: capacityLimit,
	}); err != nil {
		t.Fatalf("enrollment after decommission: %v", err)
	}
}
