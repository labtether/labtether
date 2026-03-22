package resources

// canonical_persistence.go — standalone persistence helpers for the canonical
// model. These functions encapsulate the heartbeat and connector-snapshot
// write paths that were previously apiServer methods in cmd/labtether.
//
// All store interactions are explicit parameters so the functions remain
// testable without constructing an apiServer.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/modelmap"
	"github.com/labtether/labtether/internal/modelregistry"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
)

// PersistCanonicalHeartbeat writes canonical model state for a single
// heartbeat. It is a no-op when store is nil.
func PersistCanonicalHeartbeat(
	store persistence.CanonicalModelStore,
	assetEntry assets.Asset,
	source string,
	at time.Time,
) {
	if store == nil {
		return
	}

	provider := CanonicalProviderInstanceForHeartbeat(assetEntry, source, at)
	providerEntry, err := store.UpsertProviderInstance(provider)
	if err != nil {
		securityruntime.Logf("canonical heartbeat: provider upsert failed for %s: %v", provider.ID, err)
		return
	}

	externalRefs := CanonicalExternalRefsFromAsset(assetEntry, providerEntry.ID)
	if err := store.ReplaceResourceExternalRefs(assetEntry.ID, externalRefs); err != nil {
		securityruntime.Logf("canonical heartbeat: external refs update failed for %s: %v", assetEntry.ID, err)
	}

	existingCapabilityIDs := []string{}
	if existing, ok, err := store.GetCapabilitySet("resource", assetEntry.ID); err == nil && ok {
		existingCapabilityIDs = CapabilityIDsFromSet(existing)
	} else if err != nil {
		securityruntime.Logf("canonical heartbeat: capability read failed for %s: %v", assetEntry.ID, err)
	}

	inferredCapabilityIDs := InferCapabilityIDsFromAssetMetadata(assetEntry)
	mergedCapabilityIDs := MergeCapabilityIDs(existingCapabilityIDs, inferredCapabilityIDs)
	if len(inferredCapabilityIDs) > 0 {
		if _, err := store.UpsertCapabilitySet(model.CapabilitySet{
			SubjectType:  "resource",
			SubjectID:    assetEntry.ID,
			Capabilities: CapabilitySpecsFromIDs(mergedCapabilityIDs),
			UpdatedAt:    at,
		}); err != nil {
			securityruntime.Logf("canonical heartbeat: capability set upsert failed for %s: %v", assetEntry.ID, err)
		}
	}

	resource := CanonicalResourceFromAsset(assetEntry, providerEntry.ID)
	binding := modelregistry.ResolveTemplateBinding(resource, mergedCapabilityIDs, at)
	if _, err := store.UpsertTemplateBinding(binding); err != nil {
		securityruntime.Logf("canonical heartbeat: template binding upsert failed for %s: %v", assetEntry.ID, err)
	}

	if _, err := store.UpsertIngestCheckpoint(model.IngestCheckpoint{
		ProviderInstanceID: providerEntry.ID,
		Stream:             "discover",
		Cursor:             assetEntry.ID + "@" + assetEntry.LastSeenAt.UTC().Format(time.RFC3339Nano),
		SyncedAt:           at,
	}); err != nil {
		securityruntime.Logf("canonical heartbeat: ingest checkpoint upsert failed for provider %s: %v", providerEntry.ID, err)
	}
}

// PersistCanonicalConnectorSnapshot writes canonical model state for a full
// connector discovery run. It is a no-op when store is nil. assetStore may be
// nil, in which case the asset-existence gate is skipped. connectorRegistry may
// be nil, in which case capability synthesis falls back to a nil connector.
func PersistCanonicalConnectorSnapshot(
	store persistence.CanonicalModelStore,
	assetStore persistence.AssetStore,
	connectorRegistry *connectorsdk.Registry,
	providerID string,
	instanceKey string,
	displayName string,
	groupID string,
	connector connectorsdk.Connector,
	discovered []connectorsdk.Asset,
) {
	if store == nil {
		return
	}

	providerID = CanonicalProviderName(providerID)
	if providerID == "" {
		return
	}
	startedAt := time.Now().UTC()

	providerInstance := model.ProviderInstance{
		ID:       CanonicalProviderInstanceID(model.ProviderKindConnector, providerID, instanceKey),
		Kind:     model.ProviderKindConnector,
		Provider: providerID,
		DisplayName: canonicalFirstNonEmpty(
			strings.TrimSpace(displayName),
			strings.TrimSpace(providerID+" "+instanceKey),
			providerID,
		),
		Status:     model.ProviderStatusHealthy,
		Scope:      ProviderScopeForGroup(groupID),
		ConfigRef:  strings.TrimSpace(instanceKey),
		Metadata:   map[string]any{"instance_key": strings.TrimSpace(instanceKey)},
		LastSeenAt: startedAt,
	}
	storedProvider, err := store.UpsertProviderInstance(providerInstance)
	if err != nil {
		securityruntime.Logf("canonical snapshot: provider upsert failed for %s: %v", providerInstance.ID, err)
		return
	}

	canonicalAssets := modelmap.CanonicalizeConnectorAssets(providerID, discovered)
	if len(canonicalAssets) == 0 {
		_, _ = store.RecordReconciliationResult(model.ReconciliationResult{
			ProviderInstanceID: storedProvider.ID,
			StartedAt:          startedAt,
			FinishedAt:         time.Now().UTC(),
		})
		return
	}

	persistedAssetIDs := make(map[string]struct{}, len(canonicalAssets))
	persistedAssetsByID := make(map[string]assets.Asset, len(canonicalAssets))
	knownAssetsByID := make(map[string]assets.Asset, len(canonicalAssets))
	knownAssetsLoaded := false
	if assetStore != nil {
		listedAssets, listErr := assetStore.ListAssets()
		if listErr != nil {
			securityruntime.Logf("canonical snapshot: failed to pre-list assets: %v", listErr)
		} else {
			knownAssetsLoaded = true
			knownAssetsByID = make(map[string]assets.Asset, len(listedAssets))
			for _, assetEntry := range listedAssets {
				assetID := strings.TrimSpace(assetEntry.ID)
				if assetID == "" {
					continue
				}
				knownAssetsByID[assetID] = assetEntry
			}
		}
	}

	for _, asset := range canonicalAssets {
		assetID := strings.TrimSpace(asset.ID)
		if assetID == "" {
			continue
		}
		if assetStore != nil {
			if knownAssetsLoaded {
				if assetEntry, ok := knownAssetsByID[assetID]; ok {
					persistedAssetsByID[assetID] = assetEntry
				} else {
					continue
				}
			} else {
				assetEntry, ok, getErr := assetStore.GetAsset(assetID)
				if getErr != nil {
					securityruntime.Logf("canonical snapshot: failed to load asset %s: %v", assetID, getErr)
					continue
				}
				if !ok {
					continue
				}
				persistedAssetsByID[assetID] = assetEntry
			}
		}
		persistedAssetIDs[assetID] = struct{}{}

		ref := model.ExternalRef{
			ProviderInstanceID: storedProvider.ID,
			ExternalID:         CanonicalExternalID(assetID, asset.Metadata),
			ExternalType:       canonicalFirstNonEmpty(asset.Kind, asset.Type),
			ExternalParentID:   CanonicalExternalParentID(asset.Metadata),
			RawLocator:         canonicalFirstNonEmpty(asset.Metadata["url"], asset.Metadata["path"]),
		}
		if err := store.ReplaceResourceExternalRefs(assetID, []model.ExternalRef{ref}); err != nil {
			securityruntime.Logf("canonical snapshot: external ref write failed for %s: %v", assetID, err)
		}
	}

	relationships := modelmap.SynthesizeResourceRelationships(providerID, canonicalAssets)
	filteredRelationships := make([]model.ResourceRelationship, 0, len(relationships))
	for _, relationship := range relationships {
		if _, ok := persistedAssetIDs[relationship.SourceResourceID]; !ok {
			continue
		}
		if _, ok := persistedAssetIDs[relationship.TargetResourceID]; !ok {
			continue
		}
		filteredRelationships = append(filteredRelationships, relationship)
	}
	if err := store.ReplaceResourceRelationships(storedProvider.ID, filteredRelationships); err != nil {
		securityruntime.Logf("canonical snapshot: relationship replace failed for provider %s: %v", storedProvider.ID, err)
	}

	resolvedConnector := connector
	if resolvedConnector == nil && connectorRegistry != nil {
		if candidate, ok := connectorRegistry.Get(providerID); ok {
			resolvedConnector = candidate
		}
	}
	capabilitySets := modelmap.SynthesizeCapabilitySets(resolvedConnector, canonicalAssets)
	filteredCapabilitySets := make([]model.CapabilitySet, 0, len(capabilitySets))
	for _, capabilitySet := range capabilitySets {
		if strings.EqualFold(capabilitySet.SubjectType, "provider") {
			filteredCapabilitySets = append(filteredCapabilitySets, capabilitySet)
			continue
		}
		if strings.EqualFold(capabilitySet.SubjectType, "resource") {
			if _, ok := persistedAssetIDs[strings.TrimSpace(capabilitySet.SubjectID)]; ok {
				filteredCapabilitySets = append(filteredCapabilitySets, capabilitySet)
			}
		}
	}
	if err := store.ReplaceCapabilitySets(storedProvider.ID, filteredCapabilitySets); err != nil {
		securityruntime.Logf("canonical snapshot: capability set replace failed for provider %s: %v", storedProvider.ID, err)
	}

	persistedAssetIDList := make([]string, 0, len(persistedAssetIDs))
	for assetID := range persistedAssetIDs {
		persistedAssetIDList = append(persistedAssetIDList, assetID)
	}
	sort.Strings(persistedAssetIDList)

	capabilityIDsByAsset := make(map[string][]string, len(persistedAssetIDList))
	capabilitySetLimit := len(persistedAssetIDList) + len(filteredCapabilitySets) + 64
	if capabilitySetLimit < 500 {
		capabilitySetLimit = 500
	}
	if capabilitySetLimit > 5000 {
		capabilitySetLimit = 5000
	}
	if prefetchedSets, listErr := store.ListCapabilitySets(capabilitySetLimit); listErr != nil {
		securityruntime.Logf("canonical snapshot: failed to prefetch capability sets: %v", listErr)
	} else {
		for _, capabilitySet := range prefetchedSets {
			if !strings.EqualFold(strings.TrimSpace(capabilitySet.SubjectType), "resource") {
				continue
			}
			subjectID := strings.TrimSpace(capabilitySet.SubjectID)
			if _, ok := persistedAssetIDs[subjectID]; !ok {
				continue
			}
			capabilityIDsByAsset[subjectID] = CapabilityIDsFromSet(capabilitySet)
		}
	}

	for _, assetID := range persistedAssetIDList {
		if assetStore == nil {
			continue
		}
		assetEntry, ok := persistedAssetsByID[assetID]
		if !ok {
			var getErr error
			assetEntry, ok, getErr = assetStore.GetAsset(assetID)
			if getErr != nil || !ok {
				continue
			}
		}
		capabilityIDs := InferCapabilityIDsFromAssetMetadata(assetEntry)
		if prefetchedCapabilityIDs, ok := capabilityIDsByAsset[assetID]; ok {
			capabilityIDs = MergeCapabilityIDs(capabilityIDs, prefetchedCapabilityIDs)
		} else if capabilitySet, hasCapabilitySet, capabilityErr := store.GetCapabilitySet("resource", assetID); capabilityErr == nil && hasCapabilitySet {
			capabilityIDs = MergeCapabilityIDs(capabilityIDs, CapabilityIDsFromSet(capabilitySet))
		}
		binding := modelregistry.ResolveTemplateBinding(CanonicalResourceFromAsset(assetEntry, storedProvider.ID), capabilityIDs, startedAt)
		if _, err := store.UpsertTemplateBinding(binding); err != nil {
			securityruntime.Logf("canonical snapshot: template binding write failed for %s: %v", assetID, err)
		}
	}

	finishedAt := time.Now().UTC()
	if _, err := store.UpsertIngestCheckpoint(model.IngestCheckpoint{
		ProviderInstanceID: storedProvider.ID,
		Stream:             "discover",
		Cursor:             fmt.Sprintf("count=%d;ts=%d", len(canonicalAssets), finishedAt.Unix()),
		SyncedAt:           finishedAt,
	}); err != nil {
		securityruntime.Logf("canonical snapshot: checkpoint upsert failed for provider %s: %v", storedProvider.ID, err)
	}

	if _, err := store.RecordReconciliationResult(model.ReconciliationResult{
		ProviderInstanceID: storedProvider.ID,
		CreatedCount:       len(persistedAssetIDs),
		UpdatedCount:       len(filteredRelationships) + len(filteredCapabilitySets),
		StaleCount:         0,
		ErrorCount:         0,
		StartedAt:          startedAt,
		FinishedAt:         finishedAt,
	}); err != nil {
		securityruntime.Logf("canonical snapshot: reconciliation result write failed for provider %s: %v", storedProvider.ID, err)
	}
}
