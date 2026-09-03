package persistence

import (
	"errors"
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

func TestValidateVerifiedAgentRecoveryAnchor(t *testing.T) {
	valid := map[string]string{
		assets.MetadataKeyAgentDeviceFingerprint:  "LT-VERIFIED",
		assets.MetadataKeyAgentDeviceKeyAlgorithm: "ed25519",
		assets.MetadataKeyAgentIdentityVerifiedAt: "2026-09-03T01:02:03.456Z",
	}
	tests := []struct {
		name        string
		metadata    map[string]string
		fingerprint string
		algorithm   string
		wantError   bool
	}{
		{name: "valid", metadata: valid, fingerprint: "LT-VERIFIED", algorithm: "ed25519"},
		{name: "missing marker", metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint: "LT-VERIFIED", assets.MetadataKeyAgentDeviceKeyAlgorithm: "ed25519",
		}, fingerprint: "LT-VERIFIED", algorithm: "ed25519", wantError: true},
		{name: "malformed marker", metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint: "LT-VERIFIED", assets.MetadataKeyAgentDeviceKeyAlgorithm: "ed25519",
			assets.MetadataKeyAgentIdentityVerifiedAt: "not-a-time",
		}, fingerprint: "LT-VERIFIED", algorithm: "ed25519", wantError: true},
		{name: "fingerprint mismatch", metadata: valid, fingerprint: "LT-OTHER", algorithm: "ed25519", wantError: true},
		{name: "algorithm mismatch", metadata: valid, fingerprint: "LT-VERIFIED", algorithm: "rsa", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateVerifiedAgentRecoveryAnchor(test.metadata, test.fingerprint, test.algorithm)
			if test.wantError && !errors.Is(err, ErrAgentIdentityContinuityConflict) {
				t.Fatalf("error=%v, want continuity conflict", err)
			}
			if !test.wantError && err != nil {
				t.Fatalf("valid verified anchor rejected: %v", err)
			}
		})
	}
}

func TestMemoryAssetStoreManualNameOverridePersistsAcrossHeartbeats(t *testing.T) {
	store := NewMemoryAssetStore()

	_, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1",
		Type:    "host",
		Name:    "Initial Name",
		Source:  "agent",
		Status:  "online",
		Metadata: map[string]string{
			"cpu_percent": "10.0",
		},
	})
	if err != nil {
		t.Fatalf("upsert initial heartbeat: %v", err)
	}

	renamed, err := store.UpdateAsset("node-1", assets.UpdateRequest{
		Name: ptr("Manual Name"),
	})
	if err != nil {
		t.Fatalf("rename asset: %v", err)
	}
	if renamed.Name != "Manual Name" {
		t.Fatalf("renamed name = %q, want %q", renamed.Name, "Manual Name")
	}
	if renamed.Metadata[assets.MetadataKeyNameOverride] != "Manual Name" {
		t.Fatalf("name override metadata = %q, want %q", renamed.Metadata[assets.MetadataKeyNameOverride], "Manual Name")
	}

	updated, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1",
		Type:    "host",
		Name:    "Heartbeat Name",
		Source:  "agent",
		Status:  "online",
		Metadata: map[string]string{
			"cpu_percent": "42.0",
		},
	})
	if err != nil {
		t.Fatalf("upsert second heartbeat: %v", err)
	}

	if updated.Name != "Manual Name" {
		t.Fatalf("asset name after heartbeat = %q, want manual override %q", updated.Name, "Manual Name")
	}
	if updated.Metadata[assets.MetadataKeyNameOverride] != "Manual Name" {
		t.Fatalf("name override metadata after heartbeat = %q, want %q", updated.Metadata[assets.MetadataKeyNameOverride], "Manual Name")
	}
	if updated.Metadata["cpu_percent"] != "42.0" {
		t.Fatalf("expected latest heartbeat metadata to be preserved, got cpu_percent=%q", updated.Metadata["cpu_percent"])
	}
}

func TestMemoryAssetStoreGroupPersistsAcrossHeartbeatsWithoutGroupID(t *testing.T) {
	store := NewMemoryAssetStore()

	_, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1",
		Type:    "host",
		Name:    "Initial Name",
		Source:  "agent",
		GroupID: "group-a",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("upsert initial heartbeat: %v", err)
	}

	updated, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1",
		Type:    "host",
		Name:    "Refreshed Name",
		Source:  "agent",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("upsert second heartbeat: %v", err)
	}

	if updated.GroupID != "group-a" {
		t.Fatalf("group_id after heartbeat = %q, want %q", updated.GroupID, "group-a")
	}
}

func TestMemoryAssetStoreBackfillsAndPreservesAgentIdentityAnchor(t *testing.T) {
	store := NewMemoryAssetStore()
	const (
		assetID            = "identity-node"
		trustedFingerprint = "LT-TRUSTED-FINGERPRINT"
		trustedAlgorithm   = "ed25519"
		verifiedAt         = "2026-07-14T09:00:00Z"
	)

	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID,
		Type:    "host",
		Name:    "Identity Node",
		Source:  "agent",
		Status:  "online",
		Metadata: map[string]string{
			"cpu_percent": "10",
		},
	}); err != nil {
		t.Fatalf("create asset without identity anchor: %v", err)
	}

	backfilled, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID,
		Type:    "host",
		Name:    "Identity Node",
		Source:  "agent",
		Status:  "online",
		Metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint:  trustedFingerprint,
			assets.MetadataKeyAgentDeviceKeyAlgorithm: trustedAlgorithm,
			assets.MetadataKeyAgentIdentityVerifiedAt: verifiedAt,
			"cpu_percent": "20",
		},
		AllowAgentIdentityTOFU: true,
	})
	if err != nil {
		t.Fatalf("backfill identity anchor: %v", err)
	}
	assertAgentIdentityAnchor(t, backfilled, trustedFingerprint, trustedAlgorithm, "")

	conflicting, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID,
		Type:    "host",
		Name:    "Identity Node",
		Source:  "agent",
		Status:  "online",
		Metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint:  "LT-ATTACKER-FINGERPRINT",
			assets.MetadataKeyAgentDeviceKeyAlgorithm: "attacker-algorithm",
			assets.MetadataKeyAgentIdentityVerifiedAt: "2099-01-01T00:00:00Z",
			"cpu_percent": "30",
		},
	})
	if err != nil {
		t.Fatalf("apply conflicting routine heartbeat: %v", err)
	}
	assertAgentIdentityAnchor(t, conflicting, trustedFingerprint, trustedAlgorithm, "")
	if conflicting.Metadata["cpu_percent"] != "30" {
		t.Fatalf("routine metadata did not update: %+v", conflicting.Metadata)
	}

	missing, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID,
		Type:    "host",
		Name:    "Identity Node",
		Source:  "agent",
		Status:  "online",
		Metadata: map[string]string{
			"cpu_percent": "40",
		},
	})
	if err != nil {
		t.Fatalf("apply heartbeat without identity metadata: %v", err)
	}
	assertAgentIdentityAnchor(t, missing, trustedFingerprint, trustedAlgorithm, "")
}

func TestMemoryAssetStoreAllowsVerifiedAgentIdentityRotation(t *testing.T) {
	store := NewMemoryAssetStore()
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "identity-rotation-node",
		Type:    "host",
		Name:    "Identity Rotation Node",
		Source:  "agent",
		Metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint:  "LT-OLD-FINGERPRINT",
			assets.MetadataKeyAgentDeviceKeyAlgorithm: "ed25519",
		},
	}); err != nil {
		t.Fatal(err)
	}

	rotated, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "identity-rotation-node",
		Type:    "host",
		Name:    "Identity Rotation Node",
		Source:  "agent",
		Metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint:  "LT-NEW-FINGERPRINT",
			assets.MetadataKeyAgentDeviceKeyAlgorithm: "ed25519",
		},
		AllowAgentIdentityRotation: true,
	})
	if err != nil {
		t.Fatalf("rotate verified identity anchor: %v", err)
	}
	assertAgentIdentityAnchor(t, rotated, "LT-NEW-FINGERPRINT", "ed25519", "")
}

func assertAgentIdentityAnchor(t *testing.T, asset assets.Asset, fingerprint, keyAlgorithm, verifiedAt string) {
	t.Helper()
	if got := asset.Metadata[assets.MetadataKeyAgentDeviceFingerprint]; got != fingerprint {
		t.Fatalf("device fingerprint=%q, want %q", got, fingerprint)
	}
	if got := asset.Metadata[assets.MetadataKeyAgentDeviceKeyAlgorithm]; got != keyAlgorithm {
		t.Fatalf("device key algorithm=%q, want %q", got, keyAlgorithm)
	}
	if got := asset.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt]; got != verifiedAt {
		t.Fatalf("identity verified_at=%q, want %q", got, verifiedAt)
	}
}

func ptr(value string) *string {
	return &value
}
