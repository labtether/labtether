package main

// canonical_model_runtime.go — thin apiServer wrappers for canonical model
// persistence.
//
// Persistence logic lives in internal/hubapi/resources/canonical_persistence.go.
// Pure helper functions (CanonicalResourceFromAsset, InferCapabilityIDsFromAssetMetadata, etc.)
// live in internal/hubapi/resources/canonical_helpers.go.

import (
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	resourcespkg "github.com/labtether/labtether/internal/hubapi/resources"
)

// --- apiServer methods (thin wrappers around resourcespkg persistence helpers) ---

// persistCanonicalHeartbeat writes the canonical record of a committed
// heartbeat. The committed asset's source picks the provider, not the
// heartbeat's (see resourcespkg.PersistCanonicalHeartbeatForAsset).
func (s *apiServer) persistCanonicalHeartbeat(assetEntry assets.Asset, req assets.HeartbeatRequest) {
	resourcespkg.PersistCanonicalHeartbeatForAsset(s.canonicalStore, assetEntry, req, time.Now().UTC())
}

func (s *apiServer) persistCanonicalConnectorSnapshot(
	providerID string,
	instanceKey string,
	displayName string,
	groupID string,
	connector connectorsdk.Connector,
	discovered []connectorsdk.Asset,
) {
	resourcespkg.PersistCanonicalConnectorSnapshot(
		s.canonicalStore,
		s.assetStore,
		s.connectorRegistry,
		providerID,
		instanceKey,
		displayName,
		groupID,
		connector,
		discovered,
	)
}
