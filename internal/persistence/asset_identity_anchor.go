package persistence

import (
	"strings"

	"github.com/labtether/labtether/internal/assets"
)

// mergeHeartbeatIdentityAnchor applies immutable trust-on-first-use semantics
// to agent identity metadata. A routine heartbeat may backfill an absent
// fingerprint because possession of the asset bearer token authenticates that
// heartbeat, but normal WebSocket/HTTP heartbeats do not prove possession of
// the device private key. That first backfill is therefore explicitly
// bearer-authenticated TOFU and must never author the stronger verified-at
// audit marker. Once present, the fingerprint, key algorithm, and verified-at
// audit timestamp survive missing or conflicting heartbeat data.
// Only a separately verified enrollment flow may request a rotation.
func mergeHeartbeatIdentityAnchor(existing, reported map[string]string, allowRotation, allowTOFU bool) map[string]string {
	merged := cloneMetadata(reported)
	if merged == nil {
		merged = map[string]string{}
	}
	if allowRotation {
		return merged
	}
	delete(merged, assets.MetadataKeyAgentIdentityVerifiedAt)
	if len(existing) == 0 {
		if !allowTOFU {
			delete(merged, assets.MetadataKeyAgentDeviceFingerprint)
			delete(merged, assets.MetadataKeyAgentDeviceKeyAlgorithm)
		}
		return merged
	}

	existingFingerprint := strings.TrimSpace(existing[assets.MetadataKeyAgentDeviceFingerprint])
	reportedFingerprint := strings.TrimSpace(reported[assets.MetadataKeyAgentDeviceFingerprint])
	existingKeyAlgorithm := strings.TrimSpace(existing[assets.MetadataKeyAgentDeviceKeyAlgorithm])
	if existingFingerprint == "" && existingKeyAlgorithm == "" && !allowTOFU {
		delete(merged, assets.MetadataKeyAgentDeviceFingerprint)
		delete(merged, assets.MetadataKeyAgentDeviceKeyAlgorithm)
	}

	if existingFingerprint != "" {
		merged[assets.MetadataKeyAgentDeviceFingerprint] = existingFingerprint
		if existingKeyAlgorithm != "" {
			merged[assets.MetadataKeyAgentDeviceKeyAlgorithm] = existingKeyAlgorithm
		} else if !strings.EqualFold(existingFingerprint, reportedFingerprint) {
			// Do not let a mismatched fingerprint smuggle in a key algorithm that
			// would appear to describe the preserved trust anchor.
			delete(merged, assets.MetadataKeyAgentDeviceKeyAlgorithm)
		}
	} else if existingKeyAlgorithm != "" {
		// Preserve even partially populated legacy anchors. A later fingerprint
		// backfill cannot silently rewrite the previously recorded algorithm.
		merged[assets.MetadataKeyAgentDeviceKeyAlgorithm] = existingKeyAlgorithm
	}

	if verifiedAt := strings.TrimSpace(existing[assets.MetadataKeyAgentIdentityVerifiedAt]); verifiedAt != "" {
		merged[assets.MetadataKeyAgentIdentityVerifiedAt] = verifiedAt
	}
	return merged
}
