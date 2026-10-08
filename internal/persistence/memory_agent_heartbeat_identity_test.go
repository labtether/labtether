package persistence

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/enrollment"
	"testing"
	"time"
)

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
	_, _ = store.CreateEnrollmentToken(testEnrollmentTokenParams("tofu-enrollment", "tofu", now.Add(time.Hour), 1))
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

	recoveryToken, err := store.CreateEnrollmentToken(testEnrollmentTokenParams("tofu-recovery", "tofu recovery", time.Now().UTC().Add(time.Hour), 1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitAgentEnrollment(context.Background(), AgentEnrollmentCommitRequest{
		AssetID: "bearer-anchor", Hostname: "bearer-anchor", EnrollmentTokenHash: "tofu-recovery",
		AgentTokenHash: "tofu-replacement", AgentTokenExpiresAt: time.Now().UTC().Add(time.Hour),
		DeviceFingerprint: "LT-BEARER-TOFU", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	})
	if !errors.Is(err, ErrAgentIdentityContinuityConflict) {
		t.Fatalf("unverified TOFU recovery error=%v, want continuity conflict", err)
	}
	if token, valid, err := store.ValidateEnrollmentToken("tofu-recovery"); err != nil || !valid || token.ID != recoveryToken.ID || token.UseCount != 0 {
		t.Fatalf("rejected recovery mutated token: token=%+v valid=%v err=%v", token, valid, err)
	}
	if err := store.ValidateActiveAgentTokenID(context.Background(), result.AgentToken.ID, "bearer-anchor"); err != nil {
		t.Fatalf("rejected recovery invalidated original bearer: %v", err)
	}
}
