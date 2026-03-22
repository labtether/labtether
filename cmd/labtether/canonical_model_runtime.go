package main

// canonical_model_runtime.go — thin apiServer wrappers for canonical model
// persistence and pure-function aliases.
//
// Persistence logic lives in internal/hubapi/resources/canonical_persistence.go.
// Pure helper functions (CanonicalResourceFromAsset, InferCapabilityIDsFromAssetMetadata, etc.)
// live in internal/hubapi/resources/canonical_helpers.go and are aliased below for
// backward compatibility with callers inside this package.

import (
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	resourcespkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/model"
)

// --- Thin aliases so existing call-sites in this package compile unchanged ---

func canonicalResourceFromAsset(assetEntry assets.Asset, providerInstanceID string) model.Resource {
	return resourcespkg.CanonicalResourceFromAsset(assetEntry, providerInstanceID)
}

func canonicalExternalRefsFromAsset(assetEntry assets.Asset, providerInstanceID string) []model.ExternalRef {
	return resourcespkg.CanonicalExternalRefsFromAsset(assetEntry, providerInstanceID)
}

func canonicalExternalID(fallback string, metadata map[string]string) string {
	return resourcespkg.CanonicalExternalID(fallback, metadata)
}

func canonicalExternalParentID(metadata map[string]string) string {
	return resourcespkg.CanonicalExternalParentID(metadata)
}

func inferCapabilityIDsFromAssetMetadata(assetEntry assets.Asset) []string {
	return resourcespkg.InferCapabilityIDsFromAssetMetadata(assetEntry)
}

func capabilitySpecsFromIDs(ids []string) []model.CapabilitySpec {
	return resourcespkg.CapabilitySpecsFromIDs(ids)
}

func capabilityIDsFromSet(set model.CapabilitySet) []string {
	return resourcespkg.CapabilityIDsFromSet(set)
}

func mergeCapabilityIDs(values ...[]string) []string {
	return resourcespkg.MergeCapabilityIDs(values...)
}

func splitCapabilityTokens(value string) []string {
	return resourcespkg.SplitCapabilityTokens(value)
}

func hasAnyMetricSignals(metadata map[string]string) bool {
	return resourcespkg.HasAnyMetricSignals(metadata)
}

func canonicalProviderName(source string) string {
	return resourcespkg.CanonicalProviderName(source)
}

func canonicalProviderInstanceID(kind model.ProviderKind, provider, instanceKey string) string {
	return resourcespkg.CanonicalProviderInstanceID(kind, provider, instanceKey)
}

func providerStatusFromAssetStatus(status string) model.ProviderStatus {
	return resourcespkg.ProviderStatusFromAssetStatus(status)
}

func canonicalResourceStatus(status string) model.ResourceStatus {
	return resourcespkg.CanonicalResourceStatus(status)
}

func providerScopeForGroup(groupID string) model.ProviderScope {
	return resourcespkg.ProviderScopeForGroup(groupID)
}

func isAgentSource(source string) bool {
	return resourcespkg.IsAgentSource(source)
}

func dedupeSortedStrings(values []string) []string {
	return resourcespkg.DedupeSortedStrings(values)
}

// --- apiServer methods (thin wrappers around resourcespkg persistence helpers) ---

func (s *apiServer) persistCanonicalHeartbeat(assetEntry assets.Asset, req assets.HeartbeatRequest) {
	source := canonicalProviderName(firstNonEmptyString(req.Source, assetEntry.Source))
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
