package main

import (
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/model"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestPersistCanonicalHeartbeatWritesCheckpointAndBinding(t *testing.T) {
	t.Parallel()

	sut := newTestAPIServer(t)
	assetID := "linux-node-contract"
	_, err := sut.processHeartbeatRequest(assets.HeartbeatRequest{
		AssetID:  assetID,
		Type:     "host",
		Name:     "linux-node-contract",
		Source:   "agent",
		Status:   "online",
		Platform: "linux",
		Metadata: map[string]string{
			"platform":     "linux",
			"cap_services": "list,action",
			"cap_network":  "list,action",
			"cap_logs":     "stored,query",
		},
	})
	if err != nil {
		t.Fatalf("process heartbeat: %v", err)
	}

	providerInstanceID := canonicalProviderInstanceID(model.ProviderKindAgent, "agent", assetID)
	if _, ok, err := sut.canonicalStore.GetProviderInstance(providerInstanceID); err != nil {
		t.Fatalf("get provider instance: %v", err)
	} else if !ok {
		t.Fatalf("expected provider instance %s", providerInstanceID)
	}

	storedAsset, ok, err := sut.assetStore.GetAsset(assetID)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	if !ok {
		t.Fatalf("expected persisted asset %s", assetID)
	}
	if storedAsset.ResourceClass != "compute" {
		t.Fatalf("asset.ResourceClass = %q, want compute", storedAsset.ResourceClass)
	}
	if storedAsset.ResourceKind != "host" {
		t.Fatalf("asset.ResourceKind = %q, want host", storedAsset.ResourceKind)
	}
	refs, err := sut.canonicalStore.ListResourceExternalRefs(assetID)
	if err != nil {
		t.Fatalf("list external refs: %v", err)
	}
	if len(refs) == 0 {
		t.Fatalf("expected external refs for %s", assetID)
	}
	if refs[0].ProviderInstanceID != providerInstanceID {
		t.Fatalf("external ref provider_instance_id = %q, want %q", refs[0].ProviderInstanceID, providerInstanceID)
	}
	if strings.TrimSpace(refs[0].ExternalID) == "" {
		t.Fatalf("expected non-empty external ref external_id")
	}
	if refs[0].ExternalType != "host" {
		t.Fatalf("external ref external_type = %q, want host", refs[0].ExternalType)
	}

	if _, ok, err := sut.canonicalStore.GetIngestCheckpoint(providerInstanceID, "discover"); err != nil {
		t.Fatalf("get checkpoint: %v", err)
	} else if !ok {
		t.Fatalf("expected ingest checkpoint for %s", providerInstanceID)
	}
	binding, ok, err := sut.canonicalStore.GetTemplateBinding(assetID)
	if err != nil {
		t.Fatalf("get template binding: %v", err)
	}
	if !ok {
		t.Fatalf("expected template binding for %s", assetID)
	}
	if !containsTab(binding.Tabs, "services") || !containsTab(binding.Tabs, "interfaces") {
		t.Fatalf("expected capability tabs in binding, got %v", binding.Tabs)
	}
}

func TestPersistCanonicalHeartbeatUsesCommittedSource(t *testing.T) {
	t.Parallel()
	sut := newTestAPIServer(t)
	assetID := "canonical-source-authority"
	now := time.Now().UTC()
	committed := assets.Asset{
		ID: assetID, Type: "host", Name: assetID, Source: "agent", Status: "online",
		Platform: "linux", LastSeenAt: now, UpdatedAt: now, CreatedAt: now,
	}
	sut.persistCanonicalHeartbeat(committed, assets.HeartbeatRequest{
		AssetID: assetID, Source: "proxmox",
	})
	agentProviderID := canonicalProviderInstanceID(model.ProviderKindAgent, "agent", assetID)
	if _, ok, err := sut.canonicalStore.GetProviderInstance(agentProviderID); err != nil || !ok {
		t.Fatalf("committed agent provider missing ok=%v err=%v", ok, err)
	}
	forgedProviderID := canonicalProviderInstanceID(model.ProviderKindConnector, "proxmox", assetID)
	if _, ok, err := sut.canonicalStore.GetProviderInstance(forgedProviderID); err != nil || ok {
		t.Fatalf("payload-selected provider exists=%v err=%v", ok, err)
	}
}

func TestPersistCanonicalHeartbeatWritesCheckpointAndBindingByPlatform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		platform string
	}{
		{name: "linux", platform: "linux"},
		{name: "darwin", platform: "darwin"},
		{name: "windows", platform: "windows"},
		{name: "freebsd", platform: "freebsd"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sut := newTestAPIServer(t)
			assetID := "agent-node-contract-" + tc.platform
			_, err := sut.processHeartbeatRequest(assets.HeartbeatRequest{
				AssetID:  assetID,
				Type:     "host",
				Name:     "agent-node-contract-" + tc.platform,
				Source:   "agent",
				Status:   "online",
				Platform: tc.platform,
				Metadata: map[string]string{
					"platform":         tc.platform,
					"cap_services":     "list,action",
					"cap_packages":     "list,action",
					"cap_network":      "list,action",
					"cap_schedules":    "list",
					"cap_logs":         "stored,query,stream",
					"cpu_used_percent": "17.5",
				},
			})
			if err != nil {
				t.Fatalf("process heartbeat: %v", err)
			}

			providerInstanceID := canonicalProviderInstanceID(model.ProviderKindAgent, "agent", assetID)
			snapshot := collectCanonicalHeartbeatSnapshot(t, sut, providerInstanceID, assetID)
			assertCanonicalHeartbeatSnapshotFixture(t, tc.platform, snapshot)
		})
	}
}

func collectCanonicalHeartbeatSnapshot(t *testing.T, sut *apiServer, providerInstanceID, assetID string) canonicalHeartbeatSnapshot {
	t.Helper()

	providerInstance, ok, err := sut.canonicalStore.GetProviderInstance(providerInstanceID)
	if err != nil {
		t.Fatalf("get provider instance for heartbeat snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("provider instance missing for heartbeat snapshot: %s", providerInstanceID)
	}

	assetEntry, ok, err := sut.assetStore.GetAsset(assetID)
	if err != nil {
		t.Fatalf("get asset for heartbeat snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("asset missing for heartbeat snapshot: %s", assetID)
	}

	refs, err := sut.canonicalStore.ListResourceExternalRefs(assetID)
	if err != nil {
		t.Fatalf("list external refs for heartbeat snapshot: %v", err)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].ProviderInstanceID == refs[j].ProviderInstanceID {
			return refs[i].ExternalID < refs[j].ExternalID
		}
		return refs[i].ProviderInstanceID < refs[j].ProviderInstanceID
	})
	refFixtures := make([]canonicalContractExternalRefFixture, 0, len(refs))
	for _, ref := range refs {
		refFixtures = append(refFixtures, canonicalContractExternalRefFixture{
			ResourceID:         assetID,
			ProviderInstanceID: ref.ProviderInstanceID,
			ExternalID:         ref.ExternalID,
			ExternalType:       ref.ExternalType,
			ExternalParentID:   ref.ExternalParentID,
			RawLocator:         ref.RawLocator,
		})
	}

	capabilitySet, ok, err := sut.canonicalStore.GetCapabilitySet("resource", assetID)
	if err != nil {
		t.Fatalf("get capability set for heartbeat snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("capability set missing for heartbeat snapshot: %s", assetID)
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

	binding, ok, err := sut.canonicalStore.GetTemplateBinding(assetID)
	if err != nil {
		t.Fatalf("get template binding for heartbeat snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("template binding missing for heartbeat snapshot: %s", assetID)
	}
	tabs := append([]string(nil), binding.Tabs...)
	sort.Strings(tabs)
	operations := append([]string(nil), binding.Operations...)
	sort.Strings(operations)

	checkpoint, ok, err := sut.canonicalStore.GetIngestCheckpoint(providerInstanceID, "discover")
	if err != nil {
		t.Fatalf("get ingest checkpoint for heartbeat snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("ingest checkpoint missing for heartbeat snapshot provider: %s", providerInstanceID)
	}

	return canonicalHeartbeatSnapshot{
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
		Asset: canonicalContractAssetFixture{
			ID:            assetEntry.ID,
			Type:          assetEntry.Type,
			Name:          assetEntry.Name,
			Source:        assetEntry.Source,
			ResourceClass: assetEntry.ResourceClass,
			ResourceKind:  assetEntry.ResourceKind,
			Metadata:      canonicalContractCloneStringMap(assetEntry.Metadata),
			Attributes:    canonicalContractCloneAnyMap(assetEntry.Attributes),
		},
		ExternalRefs: refFixtures,
		CapabilitySet: canonicalContractCapabilitySetFixture{
			SubjectType:  strings.ToLower(strings.TrimSpace(capabilitySet.SubjectType)),
			SubjectID:    strings.TrimSpace(capabilitySet.SubjectID),
			Capabilities: capabilities,
		},
		TemplateBinding: canonicalContractTemplateBindingFixture{
			ResourceID: binding.ResourceID,
			TemplateID: binding.TemplateID,
			Tabs:       tabs,
			Operations: operations,
		},
		Checkpoint: canonicalContractCheckpointFixture{
			ProviderInstanceID: checkpoint.ProviderInstanceID,
			Stream:             checkpoint.Stream,
			Cursor:             canonicalContractNormalizeCursor(checkpoint.Cursor),
		},
	}
}
