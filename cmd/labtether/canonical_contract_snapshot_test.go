package main

import (
	"github.com/labtether/labtether/internal/connectorsdk"
	"sort"
	"strings"
	"testing"
)

func collectCanonicalContractSnapshot(t *testing.T, sut *apiServer, providerInstanceID string, discovered []connectorsdk.Asset) canonicalContractSnapshot {
	t.Helper()

	assetIDs := make([]string, 0, len(discovered))
	assetIDSet := make(map[string]struct{}, len(discovered))
	for _, asset := range discovered {
		assetID := strings.TrimSpace(asset.ID)
		if assetID == "" {
			continue
		}
		if _, exists := assetIDSet[assetID]; exists {
			continue
		}
		assetIDSet[assetID] = struct{}{}
		assetIDs = append(assetIDs, assetID)
	}
	sort.Strings(assetIDs)

	providerInstance, ok, err := sut.canonicalStore.GetProviderInstance(providerInstanceID)
	if err != nil {
		t.Fatalf("get provider instance for snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("provider instance %s missing while building snapshot", providerInstanceID)
	}

	snapshot := canonicalContractSnapshot{
		ProviderInstance: canonicalContractProviderInstanceFixture{
			ID:          providerInstance.ID,
			Kind:        providerInstance.Kind,
			Provider:    providerInstance.Provider,
			DisplayName: providerInstance.DisplayName,
			Status:      providerInstance.Status,
			Scope:       providerInstance.Scope,
			ConfigRef:   providerInstance.ConfigRef,
			Metadata:    canonicalContractCloneAnyMap(providerInstance.Metadata),
		},
	}

	for _, assetID := range assetIDs {
		assetEntry, ok, err := sut.assetStore.GetAsset(assetID)
		if err != nil {
			t.Fatalf("get asset %s for snapshot: %v", assetID, err)
		}
		if !ok {
			t.Fatalf("asset %s missing while building snapshot", assetID)
		}
		snapshot.Assets = append(snapshot.Assets, canonicalContractAssetFixture{
			ID:            assetEntry.ID,
			Type:          assetEntry.Type,
			Name:          assetEntry.Name,
			Source:        assetEntry.Source,
			ResourceClass: assetEntry.ResourceClass,
			ResourceKind:  assetEntry.ResourceKind,
			Metadata:      canonicalContractCloneStringMap(assetEntry.Metadata),
			Attributes:    canonicalContractCloneAnyMap(assetEntry.Attributes),
		})

		refs, err := sut.canonicalStore.ListResourceExternalRefs(assetID)
		if err != nil {
			t.Fatalf("list external refs for snapshot %s: %v", assetID, err)
		}
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].ProviderInstanceID == refs[j].ProviderInstanceID {
				return refs[i].ExternalID < refs[j].ExternalID
			}
			return refs[i].ProviderInstanceID < refs[j].ProviderInstanceID
		})
		for _, ref := range refs {
			snapshot.ExternalRefs = append(snapshot.ExternalRefs, canonicalContractExternalRefFixture{
				ResourceID:         assetID,
				ProviderInstanceID: ref.ProviderInstanceID,
				ExternalID:         ref.ExternalID,
				ExternalType:       ref.ExternalType,
				ExternalParentID:   ref.ExternalParentID,
				RawLocator:         ref.RawLocator,
			})
		}

		binding, ok, err := sut.canonicalStore.GetTemplateBinding(assetID)
		if err != nil {
			t.Fatalf("get template binding for snapshot %s: %v", assetID, err)
		}
		if !ok {
			t.Fatalf("template binding for %s missing while building snapshot", assetID)
		}
		tabs := append([]string(nil), binding.Tabs...)
		sort.Strings(tabs)
		operations := append([]string(nil), binding.Operations...)
		sort.Strings(operations)
		snapshot.TemplateBindings = append(snapshot.TemplateBindings, canonicalContractTemplateBindingFixture{
			ResourceID: binding.ResourceID,
			TemplateID: binding.TemplateID,
			Tabs:       tabs,
			Operations: operations,
		})
	}

	relationships, err := sut.canonicalStore.ListResourceRelationships("", 2000)
	if err != nil {
		t.Fatalf("list relationships for snapshot: %v", err)
	}
	for _, relationship := range relationships {
		if _, ok := assetIDSet[relationship.SourceResourceID]; !ok {
			continue
		}
		if _, ok := assetIDSet[relationship.TargetResourceID]; !ok {
			continue
		}
		snapshot.Relationships = append(snapshot.Relationships, canonicalContractRelationshipFixture{
			SourceResourceID: relationship.SourceResourceID,
			TargetResourceID: relationship.TargetResourceID,
			Type:             relationship.Type,
			Direction:        relationship.Direction,
			Criticality:      relationship.Criticality,
			Inferred:         relationship.Inferred,
			Confidence:       relationship.Confidence,
			Evidence:         canonicalContractCloneAnyMap(relationship.Evidence),
		})
	}
	sort.Slice(snapshot.Relationships, func(i, j int) bool {
		left := snapshot.Relationships[i]
		right := snapshot.Relationships[j]
		if left.SourceResourceID == right.SourceResourceID {
			if left.TargetResourceID == right.TargetResourceID {
				return left.Type < right.Type
			}
			return left.TargetResourceID < right.TargetResourceID
		}
		return left.SourceResourceID < right.SourceResourceID
	})

	capabilitySets, err := sut.canonicalStore.ListCapabilitySets(2000)
	if err != nil {
		t.Fatalf("list capability sets for snapshot: %v", err)
	}
	for _, capabilitySet := range capabilitySets {
		subjectType := strings.ToLower(strings.TrimSpace(capabilitySet.SubjectType))
		subjectID := strings.TrimSpace(capabilitySet.SubjectID)
		switch subjectType {
		case "provider":
			if subjectID != providerInstanceID {
				continue
			}
		case "resource":
			if _, ok := assetIDSet[subjectID]; !ok {
				continue
			}
		default:
			continue
		}
		capabilities := make([]canonicalContractCapabilityFixture, 0, len(capabilitySet.Capabilities))
		for _, capability := range capabilitySet.Capabilities {
			capabilities = append(capabilities, canonicalContractCapabilityFixture{
				ID:             capability.ID,
				Scope:          capability.Scope,
				Stability:      capability.Stability,
				SupportsDryRun: capability.SupportsDryRun,
				SupportsAsync:  capability.SupportsAsync,
				RequiresTarget: capability.RequiresTarget,
			})
		}
		sort.Slice(capabilities, func(i, j int) bool {
			return capabilities[i].ID < capabilities[j].ID
		})
		snapshot.CapabilitySets = append(snapshot.CapabilitySets, canonicalContractCapabilitySetFixture{
			SubjectType:  subjectType,
			SubjectID:    subjectID,
			Capabilities: capabilities,
		})
	}
	sort.Slice(snapshot.CapabilitySets, func(i, j int) bool {
		if snapshot.CapabilitySets[i].SubjectType == snapshot.CapabilitySets[j].SubjectType {
			return snapshot.CapabilitySets[i].SubjectID < snapshot.CapabilitySets[j].SubjectID
		}
		return snapshot.CapabilitySets[i].SubjectType < snapshot.CapabilitySets[j].SubjectType
	})

	sort.Slice(snapshot.TemplateBindings, func(i, j int) bool {
		return snapshot.TemplateBindings[i].ResourceID < snapshot.TemplateBindings[j].ResourceID
	})

	checkpoint, ok, err := sut.canonicalStore.GetIngestCheckpoint(providerInstanceID, "discover")
	if err != nil {
		t.Fatalf("get checkpoint for snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("discover checkpoint missing while building snapshot for %s", providerInstanceID)
	}
	snapshot.Checkpoint = canonicalContractCheckpointFixture{
		ProviderInstanceID: checkpoint.ProviderInstanceID,
		Stream:             checkpoint.Stream,
		Cursor:             canonicalContractNormalizeCursor(checkpoint.Cursor),
	}

	reconciliations, err := sut.canonicalStore.ListReconciliationResults(providerInstanceID, 1)
	if err != nil {
		t.Fatalf("list reconciliation results for snapshot: %v", err)
	}
	if len(reconciliations) == 0 {
		t.Fatalf("reconciliation result missing while building snapshot for %s", providerInstanceID)
	}
	snapshot.Reconciliation = canonicalContractReconciliationFixture{
		CreatedCount: reconciliations[0].CreatedCount,
		UpdatedCount: reconciliations[0].UpdatedCount,
		StaleCount:   reconciliations[0].StaleCount,
		ErrorCount:   reconciliations[0].ErrorCount,
	}

	return snapshot
}
