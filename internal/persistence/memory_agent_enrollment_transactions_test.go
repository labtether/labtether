package persistence

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/groups"
	"testing"
	"time"
)

func TestMemoryCommitAgentEnrollmentRecoveryIsAtomicAndPreservesAsset(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	_, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("initial-enrollment", "initial", now.Add(time.Hour), 10))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "node-1", Hostname: "Node 1", Platform: "linux", GroupID: "",
		EnrollmentTokenHash: "initial-enrollment", AgentTokenHash: "first-agent-token",
		AgentTokenExpiresAt: now.Add(time.Hour), DeviceFingerprint: "LT-TRUSTED",
		DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV1,
	})
	if err != nil {
		t.Fatalf("initial enrollment: %v", err)
	}
	if first.Recovery || first.Asset.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt] == "" {
		t.Fatalf("initial signed enrollment did not author verified anchor: %+v", first)
	}

	assetStore.mu.Lock()
	custom := assetStore.assets["node-1"]
	custom.Name = "Operator name"
	custom.GroupID = "operator-group"
	custom.Platform = "operator-platform"
	custom.Tags = []string{"critical"}
	custom.Metadata = cloneMetadata(custom.Metadata)
	custom.Metadata[assets.MetadataKeyNameOverride] = "Operator name"
	custom.Metadata["operator_note"] = "preserve-me"
	assetStore.assets[custom.ID] = custom
	assetStore.mu.Unlock()

	_, err = store.CreateEnrollmentToken(testEnrollmentTokenParams("recovery-enrollment", "recovery", now.Add(time.Hour), 1))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "node-1", Hostname: "attacker-name", Platform: "attacker-platform", GroupID: "attacker-group",
		EnrollmentTokenHash: "recovery-enrollment", AgentTokenHash: "second-agent-token",
		AgentTokenExpiresAt: now.Add(2 * time.Hour), DeviceFingerprint: "LT-TRUSTED",
		DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	})
	if err != nil {
		t.Fatalf("recovery: %v", err)
	}
	if !recovered.Recovery {
		t.Fatal("expected recovery result")
	}
	revokedIDs := make(map[string]bool, len(recovered.RevokedAgentTokenIDs))
	for _, tokenID := range recovered.RevokedAgentTokenIDs {
		revokedIDs[tokenID] = true
	}
	if len(revokedIDs) != 1 || !revokedIDs[first.AgentToken.ID] {
		t.Fatalf("revoked token ids=%v, want the old credential", recovered.RevokedAgentTokenIDs)
	}
	if recovered.Asset.Name != "Operator name" || recovered.Asset.GroupID != "operator-group" || recovered.Asset.Platform != "operator-platform" || recovered.Asset.Metadata["operator_note"] != "preserve-me" {
		t.Fatalf("recovery clobbered operator-managed asset: %+v", recovered.Asset)
	}
	if _, valid, _ := store.ValidateAgentToken("first-agent-token"); valid {
		t.Fatal("old bearer remains active")
	}
	if token, valid, err := store.ValidateAgentToken("second-agent-token"); err != nil || !valid || token.AssetID != "node-1" {
		t.Fatalf("replacement bearer invalid: token=%+v valid=%v err=%v", token, valid, err)
	}
}

func TestMemoryCommitAgentEnrollmentEnforcesInitialGroupScope(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	groupStore := NewMemoryGroupStore()
	validGroup, err := groupStore.CreateGroup(groups.CreateRequest{Name: "Enrollment group", Slug: "enrollment-group"})
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryEnrollmentStoreWithGroupStore(assetStore, groupStore)
	now := time.Now().UTC()

	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("missing-group-enrollment", "missing group", now.Add(time.Hour), 2)); err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "qa-windows-host", Hostname: "QAWindowsHost", Platform: "windows", GroupID: "qa",
		EnrollmentTokenHash: "missing-group-enrollment", AgentTokenHash: "missing-group-agent-token",
		AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if !errors.Is(err, ErrEnrollmentTokenScopeMismatch) {
		t.Fatalf("missing group enrollment error=%v", err)
	}
	if _, exists, err := assetStore.GetAsset("qa-windows-host"); err != nil || exists {
		t.Fatalf("rejected group enrollment created asset: exists=%v err=%v", exists, err)
	}
	if token, valid, err := store.ValidateEnrollmentToken("missing-group-enrollment"); err != nil || !valid || token.UseCount != 0 {
		t.Fatalf("missing group token state: token=%+v valid=%v err=%v", token, valid, err)
	}
	if token, valid, err := store.ValidateAgentToken("missing-group-agent-token"); err != nil || valid {
		t.Fatalf("missing group agent token: token=%+v valid=%v err=%v", token, valid, err)
	}

	if _, err := store.CreateEnrollmentToken(testGroupEnrollmentTokenParams("valid-group-enrollment", validGroup.ID, now.Add(time.Hour), 2)); err != nil {
		t.Fatal(err)
	}
	validGroupResult, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "grouped-agent", Hostname: "Grouped Agent", Platform: "linux",
		EnrollmentTokenHash: "valid-group-enrollment", AgentTokenHash: "valid-group-agent-token",
		AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("valid group enrollment: %v", err)
	}
	if validGroupResult.Asset.GroupID != validGroup.ID {
		t.Fatalf("valid group=%q, want %q", validGroupResult.Asset.GroupID, validGroup.ID)
	}
}

func TestMemoryAssetScopedEnrollmentRejectsWrongAssetWithoutConsumption(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	if _, err := store.CreateEnrollmentToken(testAssetEnrollmentTokenParams("asset-scope", "intended-node", now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}

	_, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "other-node", Hostname: "other-node", EnrollmentTokenHash: "asset-scope",
		AgentTokenHash: "wrong-agent-token", AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if !errors.Is(err, ErrEnrollmentTokenScopeMismatch) {
		t.Fatalf("wrong asset error=%v", err)
	}
	if _, exists, err := assetStore.GetAsset("other-node"); err != nil || exists {
		t.Fatalf("wrong asset was created: exists=%v err=%v", exists, err)
	}
	if token, valid, err := store.ValidateEnrollmentToken("asset-scope"); err != nil || !valid || token.UseCount != 0 {
		t.Fatalf("rejected asset scope token=%+v valid=%v err=%v", token, valid, err)
	}
	if _, valid, err := store.ValidateAgentToken("wrong-agent-token"); err != nil || valid {
		t.Fatalf("rejected asset scope issued bearer: valid=%v err=%v", valid, err)
	}

	result, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "intended-node", Hostname: "intended-node", EnrollmentTokenHash: "asset-scope",
		AgentTokenHash: "right-agent-token", AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil || result.Asset.ID != "intended-node" {
		t.Fatalf("intended asset enrollment result=%+v err=%v", result, err)
	}
}

func TestMemoryRecoveryScopeMismatchPreservesBearerAndToken(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("initial", "initial", now.Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "recovery-node", Hostname: "recovery-node", EnrollmentTokenHash: "initial",
		AgentTokenHash: "old-bearer", AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-RECOVERY", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateEnrollmentToken(testAssetEnrollmentTokenParams("wrong-recovery", "other-node", now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	_, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "recovery-node", Hostname: "recovery-node", EnrollmentTokenHash: "wrong-recovery",
		AgentTokenHash: "new-bearer", AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-RECOVERY", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	})
	if !errors.Is(err, ErrEnrollmentTokenScopeMismatch) {
		t.Fatalf("recovery scope error=%v", err)
	}
	if _, valid, _ := store.ValidateAgentToken("old-bearer"); !valid {
		t.Fatal("scope mismatch revoked the old bearer")
	}
	if token, valid, err := store.ValidateEnrollmentToken("wrong-recovery"); err != nil || !valid || token.UseCount != 0 {
		t.Fatalf("scope mismatch consumed recovery token: token=%+v valid=%v err=%v", token, valid, err)
	}
}

func TestMemoryRecoveryRejectsMultiUseWithoutConsumingOrRotating(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	_, _ = store.CreateEnrollmentToken(testEnrollmentTokenParams("first-enrollment", "first", now.Add(time.Hour), 1))
	_, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "node-1", Hostname: "node-1", EnrollmentTokenHash: "first-enrollment",
		AgentTokenHash: "old-agent-token", AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-TRUSTED", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	multi, _ := store.CreateEnrollmentToken(testEnrollmentTokenParams("multi-enrollment", "multi", now.Add(time.Hour), 2))
	_, err = store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "node-1", Hostname: "node-1", EnrollmentTokenHash: "multi-enrollment",
		AgentTokenHash: "new-agent-token", AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-TRUSTED", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	})
	if !errors.Is(err, ErrRecoveryRequiresSingleUseToken) {
		t.Fatalf("multi-use recovery error=%v", err)
	}
	tokens, _ := store.ListEnrollmentTokens(10)
	for _, token := range tokens {
		if token.ID == multi.ID && token.UseCount != 0 {
			t.Fatalf("rejected recovery consumed enrollment token: %+v", token)
		}
	}
	if _, valid, _ := store.ValidateAgentToken("old-agent-token"); !valid {
		t.Fatal("rejected recovery revoked the old bearer")
	}
}

func TestMemoryEnrollmentRollbackOnDuplicateAgentHash(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	if _, err := store.CreateAgentToken("other", "duplicate-hash", "test", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	token, _ := store.CreateEnrollmentToken(testEnrollmentTokenParams("enrollment", "rollback", now.Add(time.Hour), 1))
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "node-rollback", Hostname: "node-rollback", EnrollmentTokenHash: "enrollment",
		AgentTokenHash: "duplicate-hash", AgentTokenExpiresAt: now.Add(time.Hour),
	}); err == nil {
		t.Fatal("expected duplicate hash failure")
	}
	if _, exists, _ := assetStore.GetAsset("node-rollback"); exists {
		t.Fatal("failed enrollment created an asset")
	}
	tokens, _ := store.ListEnrollmentTokens(10)
	for _, got := range tokens {
		if got.ID == token.ID && got.UseCount != 0 {
			t.Fatalf("failed enrollment consumed token: %+v", got)
		}
	}
}

func TestMemoryPreparedApprovalLifecycleAndOrphanCleanup(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	prepared, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: "node-approval", AgentTokenHash: "prepared-hash", PreparedTokenExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Status != "pending" {
		t.Fatalf("prepared status=%q", prepared.Status)
	}
	if _, valid, _ := store.ValidateAgentToken("prepared-hash"); valid {
		t.Fatal("prepared credential validated before finalization")
	}
	activeExpiry := now.Add(24 * time.Hour)
	if _, err := store.FinalizeAgentApproval(ctx, AgentApprovalFinalizeRequest{
		PreparedTokenID: prepared.ID, AssetID: "node-approval", Hostname: "node-approval", Platform: "linux",
		DeviceFingerprint: "LT-APPROVED", DeviceKeyAlgorithm: "ed25519", AgentTokenExpiresAt: activeExpiry,
	}); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	active, valid, err := store.ValidateAgentToken("prepared-hash")
	if err != nil || !valid || !active.ExpiresAt.Equal(activeExpiry) {
		t.Fatalf("finalized credential invalid: token=%+v valid=%v err=%v", active, valid, err)
	}

	orphan, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: "orphan", AgentTokenHash: "orphan-hash", PreparedTokenExpiresAt: time.Now().UTC().Add(10 * time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := store.FinalizeAgentApproval(ctx, AgentApprovalFinalizeRequest{
		PreparedTokenID: orphan.ID, AssetID: "orphan", DeviceFingerprint: "LT-ORPHAN", DeviceKeyAlgorithm: "ed25519", AgentTokenExpiresAt: activeExpiry,
	}); !errors.Is(err, ErrPreparedAgentApprovalNotFound) {
		t.Fatalf("expired prepared token finalized: %v", err)
	}
	_, deleted, err := store.DeleteDeadTokens()
	if err != nil || deleted != 1 {
		t.Fatalf("orphan cleanup deleted=%d err=%v", deleted, err)
	}
}

func TestPreparedTokenExpiryIsBounded(t *testing.T) {
	store := NewMemoryEnrollmentStore(NewMemoryAssetStore())
	prepared, err := store.PrepareAgentApproval(context.Background(), AgentApprovalPrepareRequest{
		AssetID: "node", AgentTokenHash: auth.HashToken("raw"), PreparedTokenExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if remaining := time.Until(prepared.ExpiresAt); remaining > maxPreparedAgentApprovalTTL+time.Second {
		t.Fatalf("prepared credential TTL not bounded: %s", remaining)
	}
}

func TestMemoryPreparedApprovalRejectsExistingStableAsset(t *testing.T) {
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "collision-node", Type: "node", Name: "Existing", Source: "agent",
		Metadata: map[string]string{assets.MetadataKeyAgentDeviceFingerprint: "LT-EXISTING"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareAgentApproval(context.Background(), AgentApprovalPrepareRequest{
		AssetID: "collision-node", AgentTokenHash: "collision-hash", PreparedTokenExpiresAt: time.Now().UTC().Add(time.Minute),
	}); !errors.Is(err, ErrAgentApprovalAssetConflict) {
		t.Fatalf("existing stable asset approval error=%v", err)
	}
	if tokens, _ := store.ListAgentTokens(10); len(tokens) != 0 {
		t.Fatalf("collision prepared a credential: %+v", tokens)
	}
}

func TestMemoryRecoveryRejectsEnrollmentTokenIssuedBeforeLatestRotation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEnrollmentStore(NewMemoryAssetStore())
	now := time.Now().UTC()
	_, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("initial-order", "initial", now.Add(time.Hour), 1))
	if err != nil {
		t.Fatal(err)
	}
	stale, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("stale-recovery-order", "stale", now.Add(time.Hour), 1))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "ordered-node", Hostname: "ordered-node", EnrollmentTokenHash: "initial-order",
		AgentTokenHash: "ordered-old-agent", AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-ORDERED", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "ordered-node", Hostname: "ordered-node", EnrollmentTokenHash: "stale-recovery-order",
		AgentTokenHash: "ordered-stale-agent", AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-ORDERED", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	})
	if !errors.Is(err, ErrEnrollmentTokenPredatesRotation) {
		t.Fatalf("stale recovery error=%v", err)
	}
	if token, valid, err := store.ValidateEnrollmentToken("stale-recovery-order"); err != nil || !valid || token.ID != stale.ID || token.UseCount != 0 {
		t.Fatalf("stale token mutated: token=%+v valid=%v err=%v", token, valid, err)
	}
	if err := store.ValidateActiveAgentTokenID(ctx, first.AgentToken.ID, "ordered-node"); err != nil {
		t.Fatalf("stale recovery invalidated active token: %v", err)
	}

	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("fresh-recovery-order", "fresh", now.Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "ordered-node", Hostname: "ordered-node", EnrollmentTokenHash: "fresh-recovery-order",
		AgentTokenHash: "ordered-fresh-agent", AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-ORDERED", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	}); err != nil {
		t.Fatalf("fresh recovery: %v", err)
	}
}

func TestMemoryPreparedApprovalReservesFleetCapacity(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEnrollmentStore(NewMemoryAssetStore())
	first, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: "reserved-first", AgentTokenHash: "reserved-first-hash",
		PreparedTokenExpiresAt: time.Now().UTC().Add(time.Minute), MaxEnrolledAgents: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: "reserved-second", AgentTokenHash: "reserved-second-hash",
		PreparedTokenExpiresAt: time.Now().UTC().Add(time.Minute), MaxEnrolledAgents: 1,
	}); !errors.Is(err, ErrAgentFleetCapacityReached) {
		t.Fatalf("second reservation error=%v", err)
	}
	if err := store.CancelAgentApproval(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: "reserved-second", AgentTokenHash: "reserved-second-fresh-hash",
		PreparedTokenExpiresAt: time.Now().UTC().Add(time.Minute), MaxEnrolledAgents: 1,
	}); err != nil {
		t.Fatalf("reservation after cancellation: %v", err)
	}
}
