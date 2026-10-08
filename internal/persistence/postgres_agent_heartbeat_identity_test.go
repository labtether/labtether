package persistence

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/groups"
	"testing"
	"time"
)

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

func TestPostgresUnverifiedHeartbeatAnchorCannotAuthorizeRecovery(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "unverified-recovery-" + suffix
	initialEnrollmentHash := "unverified-initial-enrollment-" + suffix
	recoveryEnrollmentHash := "unverified-recovery-enrollment-" + suffix
	initialAgentHash := "unverified-initial-agent-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_hash = ANY($1)`, []string{initialEnrollmentHash, recoveryEnrollmentHash})
	})

	now := time.Now().UTC()
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(initialEnrollmentHash, "initial", now.Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: initialEnrollmentHash,
		AgentTokenHash: initialAgentHash, AgentTokenExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	anchored, err := store.CommitAuthenticatedAgentHeartbeat(ctx, first.AgentToken.ID, assets.HeartbeatRequest{
		AssetID: assetID, Type: "node", Name: assetID, Source: "agent",
		Metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint:  "LT-PG-BEARER-TOFU",
			assets.MetadataKeyAgentDeviceKeyAlgorithm: "ed25519",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if anchored.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt] != "" {
		t.Fatalf("bearer heartbeat authored verified marker: %+v", anchored.Metadata)
	}
	recoveryToken, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(recoveryEnrollmentHash, "recovery", now.Add(time.Hour), 1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
		AssetID: assetID, Hostname: assetID, EnrollmentTokenHash: recoveryEnrollmentHash,
		AgentTokenHash: "unverified-replacement-" + suffix, AgentTokenExpiresAt: now.Add(time.Hour),
		DeviceFingerprint: "LT-PG-BEARER-TOFU", DeviceKeyAlgorithm: "ed25519", DeviceProofVersion: enrollment.DeviceProofVersionV2,
	})
	if !errors.Is(err, ErrAgentIdentityContinuityConflict) {
		t.Fatalf("unverified TOFU recovery error=%v, want continuity conflict", err)
	}
	var useCount int
	if err := store.pool.QueryRow(ctx, `SELECT use_count FROM enrollment_tokens WHERE id = $1`, recoveryToken.ID).Scan(&useCount); err != nil || useCount != 0 {
		t.Fatalf("rejected recovery use_count=%d err=%v", useCount, err)
	}
	if err := store.ValidateActiveAgentTokenID(ctx, first.AgentToken.ID, assetID); err != nil {
		t.Fatalf("rejected recovery invalidated original bearer: %v", err)
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
	if _, err := hubA.CreateEnrollmentToken(testEnrollmentTokenParams(enrollmentHash, "cross-hub", time.Now().UTC().Add(time.Hour), 1)); err != nil {
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
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(initialHash, "initial", now.Add(time.Hour), 1)); err != nil {
		t.Fatal(err)
	}
	stale, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(staleHash, "stale", now.Add(time.Hour), 1))
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
	if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(freshHash, "fresh", now.Add(time.Hour), 1)); err != nil {
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
