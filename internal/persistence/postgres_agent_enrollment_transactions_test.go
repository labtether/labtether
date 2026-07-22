package persistence

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/groups"
)

func TestPostgresAgentEnrollmentTransactionParity(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "agent-enrollment-tx-" + suffix
	initialEnrollmentHash := "initial-enrollment-" + suffix
	recoveryEnrollmentHash := "recovery-enrollment-" + suffix
	multiEnrollmentHash := "multi-enrollment-" + suffix
	firstAgentHash := "first-agent-" + suffix
	secondAgentHash := "second-agent-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_hash = ANY($1)`, []string{initialEnrollmentHash, recoveryEnrollmentHash, multiEnrollmentHash})
	})

	now := time.Now().UTC()
	if _, err := store.CreateEnrollmentToken(initialEnrollmentHash, "initial", now.Add(time.Hour), 10); err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: "Initial name", Platform: "linux",
		EnrollmentTokenHash: initialEnrollmentHash, AgentTokenHash: firstAgentHash,
		AgentTokenExpiresAt: now.Add(time.Hour), DeviceFingerprint: "LT-PG-TRUSTED",
		DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV1,
	})
	if err != nil {
		t.Fatalf("initial commit: %v", err)
	}
	if first.Asset.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt] == "" {
		t.Fatal("signed initial enrollment lacks server-authored verified_at")
	}
	if _, err := store.pool.Exec(ctx,
		`UPDATE assets SET name = 'Operator name', platform = 'operator-platform', tags = '["critical"]'::jsonb,
		 metadata = metadata || '{"name_override":"Operator name","operator_note":"preserve-me"}'::jsonb
		 WHERE id = $1`, assetID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateEnrollmentToken(recoveryEnrollmentHash, "recovery", now.Add(time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: "Attacker name", Platform: "attacker-platform",
		EnrollmentTokenHash: recoveryEnrollmentHash, AgentTokenHash: secondAgentHash,
		AgentTokenExpiresAt: now.Add(2 * time.Hour), DeviceFingerprint: "LT-PG-TRUSTED",
		DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	})
	if err != nil {
		t.Fatalf("recovery commit: %v", err)
	}
	if recovered.Asset.Name != "Operator name" || recovered.Asset.Platform != "operator-platform" || recovered.Asset.Metadata["operator_note"] != "preserve-me" {
		t.Fatalf("recovery clobbered asset: %+v", recovered.Asset)
	}
	if _, valid, _ := store.ValidateAgentToken(firstAgentHash); valid {
		t.Fatal("old PG bearer remained active")
	}
	if _, valid, err := store.ValidateAgentToken(secondAgentHash); err != nil || !valid {
		t.Fatalf("new PG bearer valid=%v err=%v", valid, err)
	}

	multi, err := store.CreateEnrollmentToken(multiEnrollmentHash, "multi", now.Add(time.Hour), 2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: multiEnrollmentHash,
		AgentTokenHash: "third-agent-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-PG-TRUSTED", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	})
	if !errors.Is(err, ErrRecoveryRequiresSingleUseToken) {
		t.Fatalf("multi-use PG recovery error=%v", err)
	}
	var useCount int
	if err := store.pool.QueryRow(ctx, `SELECT use_count FROM enrollment_tokens WHERE id = $1`, multi.ID).Scan(&useCount); err != nil || useCount != 0 {
		t.Fatalf("rejected PG recovery use_count=%d err=%v", useCount, err)
	}
}

func TestPostgresCommitAgentEnrollmentResolvesInitialGroupPlacement(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	missingAssetID := "missing-group-agent-" + suffix
	validAssetID := "valid-group-agent-" + suffix
	missingEnrollmentHash := "missing-group-enrollment-" + suffix
	validEnrollmentHash := "valid-group-enrollment-" + suffix
	missingAgentHash := "missing-group-agent-token-" + suffix
	validAgentHash := "valid-group-agent-token-" + suffix
	validGroup, err := store.CreateGroup(groups.CreateRequest{Name: "Enrollment group " + suffix, Slug: "enrollment-group-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = ANY($1)`, []string{missingAssetID, validAssetID})
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = ANY($1)`, []string{missingAssetID, validAssetID})
		_, _ = store.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_hash = ANY($1)`, []string{missingEnrollmentHash, validEnrollmentHash})
		_ = store.DeleteGroup(validGroup.ID)
	})

	now := time.Now().UTC()
	if _, err := store.CreateEnrollmentToken(missingEnrollmentHash, "missing group", now.Add(time.Hour), 2); err != nil {
		t.Fatal(err)
	}
	missingResult, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: missingAssetID, Hostname: "QAWindowsHost", Platform: "windows", GroupID: "qa-missing-" + suffix,
		EnrollmentTokenHash: missingEnrollmentHash, AgentTokenHash: missingAgentHash,
		AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("missing group enrollment: %v", err)
	}
	if missingResult.Asset.GroupID != "" {
		t.Fatalf("missing group was persisted: %q", missingResult.Asset.GroupID)
	}
	var persistedMissingGroup *string
	if err := store.pool.QueryRow(ctx, `SELECT group_id FROM assets WHERE id = $1`, missingAssetID).Scan(&persistedMissingGroup); err != nil {
		t.Fatal(err)
	}
	if persistedMissingGroup != nil {
		t.Fatalf("missing group column=%q, want NULL", *persistedMissingGroup)
	}
	if token, valid, err := store.ValidateEnrollmentToken(missingEnrollmentHash); err != nil || !valid || token.UseCount != 1 {
		t.Fatalf("missing group token state: token=%+v valid=%v err=%v", token, valid, err)
	}
	if token, valid, err := store.ValidateAgentToken(missingAgentHash); err != nil || !valid || token.AssetID != missingAssetID {
		t.Fatalf("missing group agent token: token=%+v valid=%v err=%v", token, valid, err)
	}

	if _, err := store.CreateEnrollmentToken(validEnrollmentHash, "valid group", now.Add(time.Hour), 2); err != nil {
		t.Fatal(err)
	}
	validResult, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: validAssetID, Hostname: "Grouped Agent", Platform: "linux", GroupID: validGroup.ID,
		EnrollmentTokenHash: validEnrollmentHash, AgentTokenHash: validAgentHash,
		AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("valid group enrollment: %v", err)
	}
	if validResult.Asset.GroupID != validGroup.ID {
		t.Fatalf("valid group=%q, want %q", validResult.Asset.GroupID, validGroup.ID)
	}
}

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
	_, _ = store.CreateEnrollmentToken(enrollmentHash, "race", now.Add(time.Hour), 1)
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

func TestPostgresPreparedApprovalLifecycle(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "prepared-approval-" + suffix
	tokenHash := "prepared-approval-token-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
	})
	prepared, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: assetID, AgentTokenHash: tokenHash, PreparedTokenExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, valid, err := store.ValidateAgentToken(tokenHash); err != nil || valid {
		t.Fatalf("pending PG token valid=%v err=%v", valid, err)
	}
	activeExpiry := time.Now().UTC().Add(24 * time.Hour)
	assetEntry, err := store.FinalizeAgentApproval(ctx, AgentApprovalFinalizeRequest{
		PreparedTokenID: prepared.ID, AssetID: assetID, Hostname: "Prepared approval", Platform: "linux",
		DeviceFingerprint: "LT-PREPARED", DeviceKeyAlgorithm: "ed25519", AgentTokenExpiresAt: activeExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if assetEntry.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt] == "" {
		t.Fatal("finalized PG approval lacks verified_at")
	}
	active, valid, err := store.ValidateAgentToken(tokenHash)
	expiryDelta := active.ExpiresAt.Sub(activeExpiry)
	if err != nil || !valid || expiryDelta < -time.Second || expiryDelta > time.Second {
		t.Fatalf("finalized PG token=%+v valid=%v err=%v", active, valid, err)
	}
}

func TestPostgresPreparedApprovalRejectsExistingStableAsset(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "approval-collision-" + suffix
	tokenHash := "approval-collision-token-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
	})
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Type: "node", Name: "Existing", Source: "agent",
		Metadata: map[string]string{assets.MetadataKeyAgentDeviceFingerprint: "LT-PG-EXISTING"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: assetID, AgentTokenHash: tokenHash, PreparedTokenExpiresAt: time.Now().UTC().Add(time.Minute),
	}); !errors.Is(err, ErrAgentApprovalAssetConflict) {
		t.Fatalf("PG collision approval error=%v", err)
	}
	var tokenCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM agent_tokens WHERE asset_id = $1`, assetID).Scan(&tokenCount); err != nil || tokenCount != 0 {
		t.Fatalf("PG collision token count=%d err=%v", tokenCount, err)
	}
}

func TestPostgresAuthenticatedHeartbeatRejectsOrphanActiveToken(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "orphan-agent-token-" + suffix
	tokenHash := "orphan-agent-hash-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
	})
	token, err := store.CreateAgentToken(assetID, tokenHash, "legacy", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAuthenticatedAgentHeartbeat(ctx, token.ID, assets.HeartbeatRequest{
		AssetID: assetID, Type: "node", Name: assetID, Source: "agent",
	}); !errors.Is(err, ErrAgentCredentialInactive) {
		t.Fatalf("PG orphan heartbeat error=%v", err)
	}
	if _, exists, err := store.GetAsset(assetID); err != nil || exists {
		t.Fatalf("PG orphan heartbeat asset exists=%v err=%v", exists, err)
	}
	if err := store.ValidateActiveAgentTokenID(ctx, token.ID, assetID); !errors.Is(err, ErrAgentCredentialInactive) {
		t.Fatalf("PG orphan token-ID validation error=%v", err)
	}
}

func TestPostgresAgentHeartbeatsPreserveOperatorGroup(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "group-bound-agent-" + suffix
	tokenHash := "group-bound-token-" + suffix
	group, err := store.CreateGroup(groups.CreateRequest{Name: "Agent group " + suffix, Slug: "agent-group-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_ = store.DeleteGroup(group.ID)
	})
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Type: "node", Name: assetID, Source: "agent", GroupID: group.ID,
	}); err != nil {
		t.Fatal(err)
	}
	token, err := store.CreateAgentToken(assetID, tokenHash, "test", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.CommitAuthenticatedAgentHeartbeat(ctx, token.ID, assets.HeartbeatRequest{
		AssetID: assetID, Type: "node", Name: assetID, Source: "manual", GroupID: "attacker-group",
	})
	if err != nil || got.GroupID != group.ID {
		t.Fatalf("PG bearer heartbeat group=%q err=%v", got.GroupID, err)
	}
	got, err = store.CommitExistingOwnerAgentHeartbeat(ctx, assets.HeartbeatRequest{
		AssetID: assetID, Type: "node", Name: assetID, Source: "manual", GroupID: "attacker-owner-group",
	})
	if err != nil || got.GroupID != group.ID {
		t.Fatalf("PG owner heartbeat group=%q err=%v", got.GroupID, err)
	}
}

func TestPostgresCrossHubTokenIDRevalidationObservesRevocation(t *testing.T) {
	hubA := newTestPostgresStore(t)
	// A second store value sharing only PostgreSQL state models another hub
	// process: no in-memory token validity cache is shared.
	hubB := &PostgresStore{pool: hubA.pool}
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "cross-hub-revalidation-" + suffix
	enrollmentHash := "cross-hub-enrollment-" + suffix
	agentHash := "cross-hub-agent-" + suffix
	t.Cleanup(func() {
		_, _ = hubA.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = hubA.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_, _ = hubA.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_hash = $1`, enrollmentHash)
	})
	if _, err := hubA.CreateEnrollmentToken(enrollmentHash, "cross-hub", time.Now().UTC().Add(time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	result, err := hubA.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: enrollmentHash,
		AgentTokenHash: agentHash, AgentTokenExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := hubB.ValidateActiveAgentTokenID(ctx, result.AgentToken.ID, assetID); err != nil {
		t.Fatalf("second hub rejected active token: %v", err)
	}
	if err := hubA.RevokeAgentToken(result.AgentToken.ID); err != nil {
		t.Fatal(err)
	}
	if err := hubB.ValidateActiveAgentTokenID(ctx, result.AgentToken.ID, assetID); !errors.Is(err, ErrAgentCredentialInactive) {
		t.Fatalf("second hub missed shared-DB revocation: %v", err)
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
	if err := <-heartbeatErr; err != nil && !errors.Is(err, ErrNotFound) {
		t.Fatalf("PG owner heartbeat error=%v", err)
	}
	if _, exists, err := store.GetAsset(assetID); err != nil || exists {
		t.Fatalf("PG owner heartbeat resurrected asset exists=%v err=%v", exists, err)
	}
}

func TestPostgresRecoveryRejectsEnrollmentTokenIssuedBeforeLatestRotation(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "ordered-recovery-" + suffix
	initialHash := "ordered-initial-enrollment-" + suffix
	staleHash := "ordered-stale-enrollment-" + suffix
	freshHash := "ordered-fresh-enrollment-" + suffix
	oldAgentHash := "ordered-old-agent-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_hash = ANY($1)`, []string{initialHash, staleHash, freshHash})
	})
	now := time.Now().UTC()
	if _, err := store.CreateEnrollmentToken(initialHash, "initial", now.Add(time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	stale, err := store.CreateEnrollmentToken(staleHash, "stale", now.Add(time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: initialHash,
		AgentTokenHash: oldAgentHash, AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-PG-ORDERED", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: staleHash,
		AgentTokenHash: "ordered-stale-agent-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-PG-ORDERED", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	})
	if !errors.Is(err, ErrEnrollmentTokenPredatesRotation) {
		t.Fatalf("stale recovery error=%v", err)
	}
	var useCount int
	if err := store.pool.QueryRow(ctx, `SELECT use_count FROM enrollment_tokens WHERE id = $1`, stale.ID).Scan(&useCount); err != nil || useCount != 0 {
		t.Fatalf("stale token use_count=%d err=%v", useCount, err)
	}
	if err := store.ValidateActiveAgentTokenID(ctx, first.AgentToken.ID, assetID); err != nil {
		t.Fatalf("old token invalidated: %v", err)
	}
	if _, err := store.CreateEnrollmentToken(freshHash, "fresh", now.Add(time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: freshHash,
		AgentTokenHash: "ordered-fresh-agent-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-PG-ORDERED", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	}); err != nil {
		t.Fatalf("fresh recovery: %v", err)
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
	if _, err := store.CreateEnrollmentToken(firstEnrollmentHash, "first", now.Add(time.Hour), 1); err != nil {
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
	secondEnrollment, err := store.CreateEnrollmentToken(secondEnrollmentHash, "second", now.Add(time.Hour), 1)
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
