package persistence

import (
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
)

func TestPostgresAssetStoreBackfillsAndPreservesAgentIdentityAnchor(t *testing.T) {
	store := newTestPostgresStore(t)
	assetID := fmt.Sprintf("identity-anchor-%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = store.DeleteAsset(assetID) })

	upsert := func(metadata map[string]string, allowTOFU, allowRotation bool) assets.Asset {
		t.Helper()
		asset, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID:                    assetID,
			Type:                       "host",
			Name:                       "Postgres Identity Node",
			Source:                     "agent",
			Status:                     "online",
			Platform:                   "linux",
			Metadata:                   metadata,
			AllowAgentIdentityTOFU:     allowTOFU,
			AllowAgentIdentityRotation: allowRotation,
		})
		if err != nil {
			t.Fatalf("upsert asset heartbeat: %v", err)
		}
		return asset
	}

	upsert(map[string]string{"cpu_percent": "10"}, false, false)
	const (
		trustedFingerprint = "LT-POSTGRES-TRUSTED"
		trustedAlgorithm   = "ed25519"
		verifiedAt         = "2026-07-14T09:00:00Z"
	)
	backfilled := upsert(map[string]string{
		assets.MetadataKeyAgentDeviceFingerprint:  trustedFingerprint,
		assets.MetadataKeyAgentDeviceKeyAlgorithm: trustedAlgorithm,
		assets.MetadataKeyAgentIdentityVerifiedAt: verifiedAt,
		"cpu_percent": "20",
	}, true, false)
	assertAgentIdentityAnchor(t, backfilled, trustedFingerprint, trustedAlgorithm, "")

	conflicting := upsert(map[string]string{
		assets.MetadataKeyAgentDeviceFingerprint:  "LT-POSTGRES-ATTACKER",
		assets.MetadataKeyAgentDeviceKeyAlgorithm: "attacker-algorithm",
		assets.MetadataKeyAgentIdentityVerifiedAt: "2099-01-01T00:00:00Z",
		"cpu_percent": "30",
	}, false, false)
	assertAgentIdentityAnchor(t, conflicting, trustedFingerprint, trustedAlgorithm, "")
	if conflicting.Metadata["cpu_percent"] != "30" {
		t.Fatalf("routine metadata did not update: %+v", conflicting.Metadata)
	}

	missing := upsert(map[string]string{"cpu_percent": "40"}, false, false)
	assertAgentIdentityAnchor(t, missing, trustedFingerprint, trustedAlgorithm, "")

	rotated := upsert(map[string]string{
		assets.MetadataKeyAgentDeviceFingerprint:  "LT-POSTGRES-ROTATED",
		assets.MetadataKeyAgentDeviceKeyAlgorithm: "ed25519",
		assets.MetadataKeyAgentIdentityVerifiedAt: "2026-07-14T10:00:00Z",
	}, false, true)
	assertAgentIdentityAnchor(t, rotated, "LT-POSTGRES-ROTATED", "ed25519", "2026-07-14T10:00:00Z")
}
