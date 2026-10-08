package persistence

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/enrollment"
	"sync"
	"testing"
	"time"
)

func TestMemoryDecommissionWinsAgainstAuthenticatedHeartbeat(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	_, _ = store.CreateEnrollmentToken(testEnrollmentTokenParams("enrollment", "test", now.Add(time.Hour), 1))
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

func TestMemoryDecommissionPermanentlyRetiresAgentIdentity(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("retire-initial", "initial", now.Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "retired-node", Hostname: "retired-node", EnrollmentTokenHash: "retire-initial",
		AgentTokenHash: "retire-agent", AgentTokenExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DecommissionAgentAsset(ctx, "retired-node"); err != nil {
		t.Fatal(err)
	}
	if err := store.DecommissionAgentAsset(ctx, "retired-node"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second decommission error=%v", err)
	}
	if _, retired := assetStore.retiredAgentIDs["retired-node"]; !retired {
		t.Fatal("decommission did not preserve an identity tombstone")
	}
	if retired, err := store.IsAgentIdentityRetired(ctx, "retired-node"); err != nil || !retired {
		t.Fatalf("retired identity lookup=%v err=%v", retired, err)
	}
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "retired-node", Name: "laundered", Type: "host", Source: "manual", Status: "online",
	}); !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("generic heartbeat reused retired identity: %v", err)
	}
	if _, err := store.CommitExistingOwnerAgentHeartbeat(ctx, assets.HeartbeatRequest{
		AssetID: "retired-node", Name: "legacy", Type: "node", Source: "agent", Status: "online",
	}); !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("legacy owner heartbeat reused retired identity: %v", err)
	}

	retryToken, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("retire-retry", "retry", now.Add(time.Hour), 2))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "retired-node", Hostname: "retired-node", EnrollmentTokenHash: "retire-retry",
		AgentTokenHash: "retire-retry-agent", AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("retired identity enrollment error=%v", err)
	}
	if token, valid, err := store.ValidateEnrollmentToken("retire-retry"); err != nil || !valid || token.ID != retryToken.ID || token.UseCount != 0 {
		t.Fatalf("retirement rejection mutated token: token=%+v valid=%v err=%v", token, valid, err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "retired-node", Hostname: "retired-node", EnrollmentTokenHash: "invalid-token",
		AgentTokenHash: "invalid-agent", AgentTokenExpiresAt: now.Add(time.Hour),
	}); !errors.Is(err, ErrEnrollmentTokenInvalid) {
		t.Fatalf("invalid token leaked retirement state: %v", err)
	}
	if _, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: "retired-node", AgentTokenHash: "retired-approval", PreparedTokenExpiresAt: now.Add(time.Minute),
	}); !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("retired identity approval error=%v", err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "replacement-node", Hostname: "replacement-node", EnrollmentTokenHash: "retire-retry",
		AgentTokenHash: "replacement-agent", AgentTokenExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("new replacement identity enrollment: %v", err)
	}
}

func TestMemoryPreparedApprovalCannotFinalizeRetiredIdentity(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	prepared, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: "prepared-retired", AgentTokenHash: "prepared-retired-token",
		PreparedTokenExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	assetStore.mu.Lock()
	assetStore.retiredAgentIDs["prepared-retired"] = time.Now().UTC()
	assetStore.mu.Unlock()

	_, err = store.FinalizeAgentApproval(ctx, AgentApprovalFinalizeRequest{
		PreparedTokenID: prepared.ID, AssetID: "prepared-retired", Hostname: "prepared-retired",
		DeviceFingerprint: "LT-RETIRED", DeviceKeyAlgorithm: "ed25519",
		AgentTokenExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("retired prepared approval error=%v", err)
	}
}

func TestMemoryNonAgentDeletionDoesNotRetireIdentity(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "manual-reusable", Name: "manual", Type: "host", Source: "manual", Status: "online",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DecommissionAgentAsset(ctx, "manual-reusable"); err != nil {
		t.Fatal(err)
	}
	if _, retired := assetStore.retiredAgentIDs["manual-reusable"]; retired {
		t.Fatal("non-agent asset deletion created an agent tombstone")
	}
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("manual-reuse-enrollment", "reuse", time.Now().UTC().Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "manual-reusable", Hostname: "manual-reusable", EnrollmentTokenHash: "manual-reuse-enrollment",
		AgentTokenHash: "manual-reuse-agent", AgentTokenExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("non-agent ID was not reusable: %v", err)
	}
}

func TestMemoryAgentSourceCannotBeDowngradedBeforeDecommission(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "sticky-agent", Name: "legacy agent", Type: "node", Source: "agent", Status: "online",
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "sticky-agent", Name: "laundered", Type: "host", Source: "manual", Status: "online",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Source != "agent" {
		t.Fatalf("generic heartbeat downgraded agent source to %q", updated.Source)
	}
	if err := store.DecommissionAgentAsset(ctx, "sticky-agent"); err != nil {
		t.Fatal(err)
	}
	if retired, err := store.IsAgentIdentityRetired(ctx, "sticky-agent"); err != nil || !retired {
		t.Fatalf("downgrade attempt escaped retirement=%v err=%v", retired, err)
	}
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "sticky-agent", Name: "reused", Type: "host", Source: "manual", Status: "online",
	}); !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("downgraded agent identity was reusable: %v", err)
	}
}

func TestMemoryCancelledApprovalDoesNotRetireLaterNonAgentAsset(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	prepared, err := store.PrepareAgentApproval(ctx, AgentApprovalPrepareRequest{
		AssetID: "cancelled-manual", AgentTokenHash: "cancelled-manual-pending",
		PreparedTokenExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CancelAgentApproval(ctx, prepared.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "cancelled-manual", Name: "manual", Type: "host", Source: "manual", Status: "online",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DecommissionAgentAsset(ctx, "cancelled-manual"); err != nil {
		t.Fatal(err)
	}
	if retired, err := store.IsAgentIdentityRetired(ctx, "cancelled-manual"); err != nil || retired {
		t.Fatalf("cancelled approval retired later manual asset=%v err=%v", retired, err)
	}
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("cancelled-manual-enrollment", "reuse", time.Now().UTC().Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: "cancelled-manual", Hostname: "cancelled-manual",
		EnrollmentTokenHash: "cancelled-manual-enrollment", AgentTokenHash: "cancelled-manual-agent",
		AgentTokenExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("cancelled approval blocked later enrollment: %v", err)
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
	if err := <-heartbeatErr; err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrAgentIdentityRetired) {
		t.Fatalf("owner heartbeat error=%v", err)
	}
	if _, exists, _ := assetStore.GetAsset("owner-race"); exists {
		t.Fatal("owner heartbeat resurrected decommissioned asset")
	}
}

func TestMemoryFleetCapacityIsDurableUntilDecommission(t *testing.T) {
	ctx := context.Background()
	assetStore := NewMemoryAssetStore()
	store := NewMemoryEnrollmentStore(assetStore)
	now := time.Now().UTC()
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("capacity-first-enrollment", "first", now.Add(time.Hour), 1)); err != nil {
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
	secondEnrollment, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("capacity-second-enrollment", "second", now.Add(time.Hour), 1))
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
