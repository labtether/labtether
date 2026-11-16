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

func (s *apiServer) persistCanonicalHeartbeat(assetEntry assets.Asset, req assets.HeartbeatRequest) {
	// The committed asset is authoritative. Heartbeat payloads are untrusted and
	// must not select a canonical provider/binding after persistence has already
	// normalized their source (for example, an agent reporting "proxmox").
	source := resourcespkg.CanonicalProviderName(firstNonEmptyString(assetEntry.Source, req.Source))
	resourcespkg.PersistCanonicalHeartbeat(s.canonicalStore, assetEntry, source, time.Now().UTC())
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
