package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/groups"
)

func TestMemoryCommitAgentEnrollmentRecoveryIsAtomicAndPreservesAsset(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	_, err := store.CreateEnrollmentToken("initial-enrollment", "initial", now.Add(time.Hour), 10)
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

	_, err = store.CreateEnrollmentToken("recovery-enrollment", "recovery", now.Add(time.Hour), 1)
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

func TestMemoryCommitAgentEnrollmentResolvesInitialGroupPlacement(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	groupStore := NewMemoryGroupStore()
	validGroup, err := groupStore.CreateGroup(groups.CreateRequest{Name: "Enrollment group", Slug: "enrollment-group"})
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryEnrollmentStoreWithGroupStore(assetStore, groupStore)
	now := time.Now().UTC()

	if _, err := store.CreateEnrollmentToken("missing-group-enrollment", "missing group", now.Add(time.Hour), 2); err != nil {
		t.Fatal(err)
	}
	missingGroupResult, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "qa-windows-host", Hostname: "QAWindowsHost", Platform: "windows", GroupID: "qa",
		EnrollmentTokenHash: "missing-group-enrollment", AgentTokenHash: "missing-group-agent-token",
		AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("missing group enrollment: %v", err)
	}
	if missingGroupResult.Asset.GroupID != "" {
		t.Fatalf("missing group was persisted: %q", missingGroupResult.Asset.GroupID)
	}
	if token, valid, err := store.ValidateEnrollmentToken("missing-group-enrollment"); err != nil || !valid || token.UseCount != 1 {
		t.Fatalf("missing group token state: token=%+v valid=%v err=%v", token, valid, err)
	}
	if token, valid, err := store.ValidateAgentToken("missing-group-agent-token"); err != nil || !valid || token.AssetID != "qa-windows-host" {
		t.Fatalf("missing group agent token: token=%+v valid=%v err=%v", token, valid, err)
	}

	if _, err := store.CreateEnrollmentToken("valid-group-enrollment", "valid group", now.Add(time.Hour), 2); err != nil {
		t.Fatal(err)
	}
	validGroupResult, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "grouped-agent", Hostname: "Grouped Agent", Platform: "linux", GroupID: validGroup.ID,
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

func TestMemoryRecoveryRejectsMultiUseWithoutConsumingOrRotating(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	_, _ = store.CreateEnrollmentToken("first-enrollment", "first", now.Add(time.Hour), 1)
	_, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "node-1", Hostname: "node-1", EnrollmentTokenHash: "first-enrollment",
		AgentTokenHash: "old-agent-token", AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-TRUSTED", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	multi, _ := store.CreateEnrollmentToken("multi-enrollment", "multi", now.Add(time.Hour), 2)
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
	token, _ := store.CreateEnrollmentToken("enrollment", "rollback", now.Add(time.Hour), 1)
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

func TestMemoryDecommissionWinsAgainstAuthenticatedHeartbeat(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	_, _ = store.CreateEnrollmentToken("enrollment", "test", now.Add(time.Hour), 1)
	result, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "node-race", Hostname: "node-race", EnrollmentTokenHash: "enrollment",
		AgentTokenHash: "agent-hash", AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	heartbeatErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		<-start
		_, err := store.CommitAuthenticatedAgentHeartbeat(ctx, result.AgentToken.ID, assets.HeartbeatRequest{
			AssetID: "node-race", Type: "node", Name: "node-race", Source: "agent", Status: "online",
		})
		heartbeatErr <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		if err := store.DecommissionAgentAsset(ctx, "node-race"); err != nil {
			t.Errorf("decommission: %v", err)
		}
	}()
	close(start)
	wg.Wait()
	if err := <-heartbeatErr; err != nil && !errors.Is(err, ErrAgentCredentialInactive) {
		t.Fatalf("heartbeat race error=%v", err)
	}
	if _, exists, _ := assetStore.GetAsset("node-race"); exists {
		t.Fatal("authenticated heartbeat resurrected decommissioned asset")
	}
	if _, valid, _ := store.ValidateAgentToken("agent-hash"); valid {
		t.Fatal("decommission left bearer active")
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

func TestMemoryAuthenticatedHeartbeatRejectsOrphanActiveToken(t *testing.T) {
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	token, err := store.CreateAgentToken("missing-asset", "orphan-agent-hash", "legacy", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAuthenticatedAgentHeartbeat(context.Background(), token.ID, assets.HeartbeatRequest{
		AssetID: "missing-asset", Type: "node", Name: "missing-asset", Source: "agent",
	}); !errors.Is(err, ErrAgentCredentialInactive) {
		t.Fatalf("orphan active token heartbeat error=%v", err)
	}
	if _, exists, _ := assetStore.GetAsset("missing-asset"); exists {
		t.Fatal("orphan active token recreated missing asset")
	}
	if err := store.ValidateActiveAgentTokenID(context.Background(), token.ID, "missing-asset"); !errors.Is(err, ErrAgentCredentialInactive) {
		t.Fatalf("orphan token-ID validation error=%v", err)
	}
}

func TestMemoryAgentHeartbeatsPreserveOperatorGroup(t *testing.T) {
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "group-bound-agent", Type: "node", Name: "group-bound-agent", Source: "agent", GroupID: "trusted-group",
	}); err != nil {
		t.Fatal(err)
	}
	token, err := store.CreateAgentToken("group-bound-agent", "group-bound-hash", "test", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.CommitAuthenticatedAgentHeartbeat(context.Background(), token.ID, assets.HeartbeatRequest{
		AssetID: "group-bound-agent", Type: "node", Name: "group-bound-agent", Source: "manual", GroupID: "attacker-group",
	})
	if err != nil || got.GroupID != "trusted-group" {
		t.Fatalf("bearer heartbeat group=%q err=%v", got.GroupID, err)
	}
	got, err = store.CommitExistingOwnerAgentHeartbeat(context.Background(), assets.HeartbeatRequest{
		AssetID: "group-bound-agent", Type: "node", Name: "group-bound-agent", Source: "manual", GroupID: "attacker-owner-group",
	})
	if err != nil || got.GroupID != "trusted-group" {
		t.Fatalf("owner heartbeat group=%q err=%v", got.GroupID, err)
	}
}

func TestMemoryOwnerHeartbeatCannotRaceDecommissionResurrection(t *testing.T) {
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "owner-race", Type: "node", Name: "owner-race", Source: "agent",
	}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	heartbeatErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		<-start
		_, err := store.CommitExistingOwnerAgentHeartbeat(context.Background(), assets.HeartbeatRequest{
			AssetID: "owner-race", Type: "node", Name: "owner-race", Source: "agent",
		})
		heartbeatErr <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		if err := store.DecommissionAgentAsset(context.Background(), "owner-race"); err != nil {
			t.Errorf("decommission: %v", err)
		}
	}()
	close(start)
	wg.Wait()
	if err := <-heartbeatErr; err != nil && !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner heartbeat error=%v", err)
	}
	if _, exists, _ := assetStore.GetAsset("owner-race"); exists {
		t.Fatal("owner heartbeat resurrected decommissioned asset")
	}
}

func TestMemoryIdentityTOFURequiresBoundAgentBearer(t *testing.T) {
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "generic-anchor", Type: "node", Name: "generic", Source: "manual",
		Metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint:  "LT-GENERIC",
			assets.MetadataKeyAgentDeviceKeyAlgorithm: "ed25519",
			assets.MetadataKeyAgentIdentityVerifiedAt: "2099-01-01T00:00:00Z",
		},
	}); err != nil {
		t.Fatal(err)
	}
	generic, _, _ := assetStore.GetAsset("generic-anchor")
	if generic.Metadata[assets.MetadataKeyAgentDeviceFingerprint] != "" || generic.Metadata[assets.MetadataKeyAgentDeviceKeyAlgorithm] != "" || generic.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt] != "" {
		t.Fatalf("generic heartbeat authored identity anchor: %+v", generic.Metadata)
	}

	now := time.Now().UTC()
	_, _ = store.CreateEnrollmentToken("tofu-enrollment", "tofu", now.Add(time.Hour), 1)
	result, err := store.CommitAgentEnrollment(context.Background(), AgentEnrollmentCommitRequest{
		AssetID: "bearer-anchor", Hostname: "bearer-anchor", EnrollmentTokenHash: "tofu-enrollment",
		AgentTokenHash: "tofu-agent", AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	anchored, err := store.CommitAuthenticatedAgentHeartbeat(context.Background(), result.AgentToken.ID, assets.HeartbeatRequest{
		AssetID: "bearer-anchor", Type: "node", Name: "bearer-anchor", Source: "manual",
		Metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint:  "LT-BEARER-TOFU",
			assets.MetadataKeyAgentDeviceKeyAlgorithm: "ed25519",
			assets.MetadataKeyAgentIdentityVerifiedAt: "2099-01-01T00:00:00Z",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if anchored.Source != "agent" {
		t.Fatalf("authenticated heartbeat changed server-owned source to %q", anchored.Source)
	}
	if anchored.Metadata[assets.MetadataKeyAgentDeviceFingerprint] != "LT-BEARER-TOFU" || anchored.Metadata[assets.MetadataKeyAgentDeviceKeyAlgorithm] != "ed25519" {
		t.Fatalf("bound bearer failed TOFU anchor: %+v", anchored.Metadata)
	}
	if anchored.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt] != "" {
		t.Fatalf("unsigned bearer TOFU authored verified_at: %+v", anchored.Metadata)
	}
}

func TestMemoryRecoveryRejectsEnrollmentTokenIssuedBeforeLatestRotation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEnrollmentStore(NewMemoryAssetStore())
	now := time.Now().UTC()
	_, err := store.CreateEnrollmentToken("initial-order", "initial", now.Add(time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := store.CreateEnrollmentToken("stale-recovery-order", "stale", now.Add(time.Hour), 1)
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

	if _, err := store.CreateEnrollmentToken("fresh-recovery-order", "fresh", now.Add(time.Hour), 1); err != nil {
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

func TestMemoryFleetCapacityIsDurableUntilDecommission(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	if _, err := store.CreateEnrollmentToken("capacity-first-enrollment", "first", now.Add(time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "capacity-first", Hostname: "capacity-first", EnrollmentTokenHash: "capacity-first-enrollment",
		AgentTokenHash: "capacity-first-agent", AgentTokenExpiresAt: now.Add(time.Hour), MaxEnrolledAgents: 1,
		DeviceFingerprint: "LT-CAPACITY", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeAgentToken(first.AgentToken.ID); err != nil {
		t.Fatal(err)
	}
	secondEnrollment, err := store.CreateEnrollmentToken("capacity-second-enrollment", "second", now.Add(time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "capacity-second", Hostname: "capacity-second", EnrollmentTokenHash: "capacity-second-enrollment",
		AgentTokenHash: "capacity-second-agent", AgentTokenExpiresAt: now.Add(time.Hour), MaxEnrolledAgents: 1,
	})
	if !errors.Is(err, ErrAgentFleetCapacityReached) {
		t.Fatalf("capacity error=%v", err)
	}
	if token, valid, err := store.ValidateEnrollmentToken("capacity-second-enrollment"); err != nil || !valid || token.ID != secondEnrollment.ID || token.UseCount != 0 {
		t.Fatalf("capacity rejection mutated token: token=%+v valid=%v err=%v", token, valid, err)
	}
	if err := store.DecommissionAgentAsset(ctx, "capacity-first"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "capacity-second", Hostname: "capacity-second", EnrollmentTokenHash: "capacity-second-enrollment",
		AgentTokenHash: "capacity-second-agent", AgentTokenExpiresAt: now.Add(time.Hour), MaxEnrolledAgents: 1,
	}); err != nil {
		t.Fatalf("enrollment after decommission: %v", err)
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
