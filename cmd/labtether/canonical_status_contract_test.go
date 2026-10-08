package main

import (
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"sort"
	"strings"
	"testing"
)

func TestStatusAggregateIncludesCanonicalPayload(t *testing.T) {
	t.Parallel()

	sut := newTestAPIServer(t)
	discovered := []connectorsdk.Asset{
		{ID: "proxmox-node-status-1", Type: "hypervisor-node", Name: "status-node", Source: "proxmox", Metadata: map[string]string{"node": "status-node"}},
		{ID: "proxmox-vm-status-101", Type: "vm", Name: "status-vm", Source: "proxmox", Metadata: map[string]string{"node": "status-node", "vmid": "101"}},
	}
	for _, asset := range discovered {
		seedCanonicalContractAsset(t, sut, asset)
	}

	connector := &canonicalContractConnector{
		id:      "proxmox",
		display: "Proxmox",
		caps: connectorsdk.Capabilities{
			DiscoverAssets: true,
			CollectMetrics: true,
			CollectEvents:  true,
			ExecuteActions: true,
		},
		actions: []connectorsdk.ActionDescriptor{{
			ID:             "vm.start",
			CanonicalID:    "workload.start",
			Name:           "Start VM",
			RequiresTarget: true,
			SupportsDryRun: true,
		}},
	}
	sut.persistCanonicalConnectorSnapshot("proxmox", "collector-status", connector.DisplayName(), "", connector, discovered)

	agentAssetID := "linux-node-status-contract"
	if _, err := sut.processHeartbeatRequest(assets.HeartbeatRequest{
		AssetID:  agentAssetID,
		Type:     "host",
		Name:     "linux-node-status-contract",
		Source:   "agent",
		Status:   "online",
		Platform: "linux",
		Metadata: map[string]string{
			"platform":         "linux",
			"cap_services":     "list,action",
			"cap_network":      "list,action",
			"cap_logs":         "stored,query",
			"cpu_used_percent": "11.5",
		},
	}); err != nil {
		t.Fatalf("process heartbeat: %v", err)
	}

	response := sut.buildStatusAggregateResponse(context.Background(), "")
	if len(response.Canonical.Registry.Capabilities) == 0 {
		t.Fatalf("expected canonical registry capabilities in status payload")
	}
	if len(response.Canonical.Providers) == 0 {
		t.Fatalf("expected canonical providers in status payload")
	}
	if _, ok := response.Canonical.TemplateBindings[agentAssetID]; !ok {
		b, _ := json.Marshal(response.Canonical.TemplateBindings)
		t.Fatalf("expected template binding for %s, got %s", agentAssetID, string(b))
	}

	snapshot := collectCanonicalStatusAggregateSnapshot(response.Canonical)
	assertCanonicalStatusAggregateSnapshotFixture(t, "default", snapshot)
}

func collectCanonicalStatusAggregateSnapshot(payload statusCanonicalPayload) canonicalStatusAggregateSnapshot {
	out := canonicalStatusAggregateSnapshot{
		Registry: canonicalStatusRegistryFixture{
			CapabilityIDs: make([]string, 0, len(payload.Registry.Capabilities)),
			OperationIDs:  make([]string, 0, len(payload.Registry.Operations)),
			MetricIDs:     make([]string, 0, len(payload.Registry.Metrics)),
			EventIDs:      make([]string, 0, len(payload.Registry.Events)),
			TemplateIDs:   make([]string, 0, len(payload.Registry.Templates)),
		},
		Providers:        make([]canonicalContractProviderInstanceFixture, 0, len(payload.Providers)),
		CapabilitySets:   make([]canonicalContractCapabilitySetFixture, 0, len(payload.CapabilitySets)),
		TemplateBindings: make([]canonicalContractTemplateBindingFixture, 0, len(payload.TemplateBindings)),
		Reconciliation:   make([]canonicalStatusReconciliationFixture, 0, len(payload.Reconciliation)),
	}

	for _, capability := range payload.Registry.Capabilities {
		out.Registry.CapabilityIDs = append(out.Registry.CapabilityIDs, strings.TrimSpace(capability.ID))
	}
	for _, operation := range payload.Registry.Operations {
		out.Registry.OperationIDs = append(out.Registry.OperationIDs, strings.TrimSpace(operation.ID))
	}
	for _, metric := range payload.Registry.Metrics {
		out.Registry.MetricIDs = append(out.Registry.MetricIDs, strings.TrimSpace(metric.ID))
	}
	for _, event := range payload.Registry.Events {
		out.Registry.EventIDs = append(out.Registry.EventIDs, strings.TrimSpace(event.ID))
	}
	for _, template := range payload.Registry.Templates {
		out.Registry.TemplateIDs = append(out.Registry.TemplateIDs, strings.TrimSpace(template.ID))
	}
	sort.Strings(out.Registry.CapabilityIDs)
	sort.Strings(out.Registry.OperationIDs)
	sort.Strings(out.Registry.MetricIDs)
	sort.Strings(out.Registry.EventIDs)
	sort.Strings(out.Registry.TemplateIDs)

	for _, provider := range payload.Providers {
		out.Providers = append(out.Providers, canonicalContractProviderInstanceFixture{
			ID:          provider.ID,
			Kind:        provider.Kind,
			Provider:    provider.Provider,
			DisplayName: provider.DisplayName,
			Status:      provider.Status,
			Scope:       provider.Scope,
			ConfigRef:   provider.ConfigRef,
			Metadata:    canonicalContractCloneAnyMap(provider.Metadata),
		})
	}
	sort.Slice(out.Providers, func(i, j int) bool {
		return out.Providers[i].ID < out.Providers[j].ID
	})

	for _, capabilitySet := range payload.CapabilitySets {
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
		out.CapabilitySets = append(out.CapabilitySets, canonicalContractCapabilitySetFixture{
			SubjectType:  strings.ToLower(strings.TrimSpace(capabilitySet.SubjectType)),
			SubjectID:    strings.TrimSpace(capabilitySet.SubjectID),
			Capabilities: capabilities,
		})
	}
	sort.Slice(out.CapabilitySets, func(i, j int) bool {
		if out.CapabilitySets[i].SubjectType == out.CapabilitySets[j].SubjectType {
			return out.CapabilitySets[i].SubjectID < out.CapabilitySets[j].SubjectID
		}
		return out.CapabilitySets[i].SubjectType < out.CapabilitySets[j].SubjectType
	})

	resourceIDs := make([]string, 0, len(payload.TemplateBindings))
	for resourceID := range payload.TemplateBindings {
		resourceIDs = append(resourceIDs, resourceID)
	}
	sort.Strings(resourceIDs)
	for _, resourceID := range resourceIDs {
		binding := payload.TemplateBindings[resourceID]
		tabs := append([]string(nil), binding.Tabs...)
		sort.Strings(tabs)
		operations := append([]string(nil), binding.Operations...)
		sort.Strings(operations)
		out.TemplateBindings = append(out.TemplateBindings, canonicalContractTemplateBindingFixture{
			ResourceID: binding.ResourceID,
			TemplateID: binding.TemplateID,
			Tabs:       tabs,
			Operations: operations,
		})
	}

	for _, result := range payload.Reconciliation {
		out.Reconciliation = append(out.Reconciliation, canonicalStatusReconciliationFixture{
			ProviderInstanceID: result.ProviderInstanceID,
			CreatedCount:       result.CreatedCount,
			UpdatedCount:       result.UpdatedCount,
			StaleCount:         result.StaleCount,
			ErrorCount:         result.ErrorCount,
		})
	}
	sort.Slice(out.Reconciliation, func(i, j int) bool {
		return out.Reconciliation[i].ProviderInstanceID < out.Reconciliation[j].ProviderInstanceID
	})

	return out
}
