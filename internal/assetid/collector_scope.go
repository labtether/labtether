package assetid

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	collectorScopeSeparator = "--ltc-"
	collectorScopeHexLength = 32
)

// CollectorScope returns a stable, opaque namespace for a persisted collector
// ID. The collector ID remains the authoritative identity; the digest keeps
// generated asset IDs compact and avoids leaking internal collector names.
func CollectorScope(collectorID string) string {
	collectorID = strings.TrimSpace(collectorID)
	if collectorID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(collectorID))
	return hex.EncodeToString(sum[:16])
}

// ScopeCollectorAssetID namespaces a connector-native asset ID by collector.
// It is idempotent for already-scoped IDs and replaces a previous valid scope
// when an ID is deliberately re-associated with another collector.
func ScopeCollectorAssetID(assetID, collectorID string) string {
	assetID = NativeCollectorAssetID(assetID)
	scope := CollectorScope(collectorID)
	if assetID == "" || scope == "" {
		return assetID
	}
	return assetID + collectorScopeSeparator + scope
}

// NativeCollectorAssetID removes a valid LabTether collector namespace suffix.
// Unrelated IDs containing "--ltc-" are left unchanged.
func NativeCollectorAssetID(assetID string) string {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return ""
	}
	separatorAt := strings.LastIndex(assetID, collectorScopeSeparator)
	if separatorAt <= 0 {
		return assetID
	}
	scope := assetID[separatorAt+len(collectorScopeSeparator):]
	if !validCollectorScope(scope) {
		return assetID
	}
	return assetID[:separatorAt]
}

// CollectorScopeFromAssetID returns the validated scope suffix, when present.
func CollectorScopeFromAssetID(assetID string) (string, bool) {
	assetID = strings.TrimSpace(assetID)
	separatorAt := strings.LastIndex(assetID, collectorScopeSeparator)
	if separatorAt <= 0 {
		return "", false
	}
	scope := assetID[separatorAt+len(collectorScopeSeparator):]
	if !validCollectorScope(scope) {
		return "", false
	}
	return scope, true
}

func validCollectorScope(scope string) bool {
	if len(scope) != collectorScopeHexLength {
		return false
	}
	_, err := hex.DecodeString(scope)
	return err == nil
}
