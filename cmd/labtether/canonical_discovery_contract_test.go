package main

import (
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPersistCanonicalConnectorSnapshotWritesCanonicalStateByProvider(t *testing.T) {
	t.Parallel()

	type expectedCanonicalAsset struct {
		id    string
		class string
		kind  string
	}

	tests := []struct {
		name                  string
		providerID            string
		discovered            []connectorsdk.Asset
		expectedAssets        []expectedCanonicalAsset
		expectRelationships   bool
		expectedResourceSetID string
	}{
		{
			name:       "docker",
			providerID: "docker",
			discovered: []connectorsdk.Asset{
				{ID: "docker-host-lab", Type: "container-host", Name: "docker-host-lab", Source: "docker", Metadata: map[string]string{"hostname": "docker-host-lab"}},
				{ID: "docker-ct-nginx", Type: "docker-container", Name: "nginx", Source: "docker", Metadata: map[string]string{"container_id": "abc123"}},
			},
			expectedAssets: []expectedCanonicalAsset{
				{id: "docker-host-lab", class: "compute", kind: "container-host"},
				{id: "docker-ct-nginx", class: "compute", kind: "docker-container"},
			},
			expectedResourceSetID: "docker-ct-nginx",
		},
		{
			name:       "portainer",
			providerID: "portainer",
			discovered: []connectorsdk.Asset{
				{ID: "portainer-endpoint-1", Type: "container-host", Name: "endpoint-1", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1"}},
				{ID: "portainer-ct-1", Type: "container", Name: "api", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1", "container_id": "portainer-ct-1"}},
			},
			expectedAssets: []expectedCanonicalAsset{
				{id: "portainer-endpoint-1", class: "compute", kind: "container-host"},
				{id: "portainer-ct-1", class: "compute", kind: "container"},
			},
			expectRelationships:   true,
			expectedResourceSetID: "portainer-ct-1",
		},
		{
			name:       "proxmox",
			providerID: "proxmox",
			discovered: []connectorsdk.Asset{
				{ID: "proxmox-node-pve01", Type: "hypervisor-node", Name: "pve01", Source: "proxmox", Metadata: map[string]string{"node": "pve01"}},
				{ID: "proxmox-vm-101", Type: "vm", Name: "vm-101", Source: "proxmox", Metadata: map[string]string{"node": "pve01", "vmid": "101"}},
			},
			expectedAssets: []expectedCanonicalAsset{
				{id: "proxmox-node-pve01", class: "compute", kind: "hypervisor-node"},
				{id: "proxmox-vm-101", class: "compute", kind: "vm"},
			},
			expectRelationships:   true,
			expectedResourceSetID: "proxmox-vm-101",
		},
		{
			name:       "pbs",
			providerID: "pbs",
			discovered: []connectorsdk.Asset{
				{ID: "pbs-root-1", Type: "storage-controller", Name: "pbs-root", Source: "pbs", Metadata: map[string]string{"hostname": "pbs-root"}},
				{ID: "pbs-datastore-main", Type: "storage-pool", Name: "main", Source: "pbs", Metadata: map[string]string{"store": "main"}},
			},
			expectedAssets: []expectedCanonicalAsset{
				{id: "pbs-root-1", class: "storage", kind: "storage-controller"},
				{id: "pbs-datastore-main", class: "storage", kind: "datastore"},
			},
			expectRelationships:   true,
			expectedResourceSetID: "pbs-datastore-main",
		},
		{
			name:       "truenas",
			providerID: "truenas",
			discovered: []connectorsdk.Asset{
				{ID: "truenas-host-nas01", Type: "nas", Name: "nas01", Source: "truenas", Metadata: map[string]string{"hostname": "nas01"}},
				{ID: "truenas-pool-main", Type: "storage-pool", Name: "main", Source: "truenas", Metadata: map[string]string{"pool_id": "main"}},
			},
			expectedAssets: []expectedCanonicalAsset{
				{id: "truenas-host-nas01", class: "storage", kind: "storage-controller"},
				{id: "truenas-pool-main", class: "storage", kind: "storage-pool"},
			},
			expectRelationships:   true,
			expectedResourceSetID: "truenas-pool-main",
		},
		{
			name:       "home assistant",
			providerID: "home-assistant",
			discovered: []connectorsdk.Asset{
				{ID: "ha-entity-light-kitchen", Type: "entity", Name: "Kitchen Light", Source: "home-assistant", Metadata: map[string]string{"entity_id": "light.kitchen", "domain": "light"}},
			},
			expectedAssets: []expectedCanonicalAsset{
				{id: "ha-entity-light-kitchen", class: "service", kind: "ha-entity"},
			},
			expectedResourceSetID: "ha-entity-light-kitchen",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sut := newTestAPIServer(t)
			expectedByID := make(map[string]expectedCanonicalAsset, len(tc.expectedAssets))
			for _, expected := range tc.expectedAssets {
				expectedByID[expected.id] = expected
			}
			for _, asset := range tc.discovered {
				seedCanonicalContractAsset(t, sut, asset)
			}

			connector := &canonicalContractConnector{
				id:      tc.providerID,
				display: "connector-" + tc.providerID,
				caps: connectorsdk.Capabilities{
					DiscoverAssets: true,
					CollectMetrics: true,
					CollectEvents:  true,
					ExecuteActions: true,
				},
				actions: []connectorsdk.ActionDescriptor{{
					ID:             "system.reboot",
					CanonicalID:    "system.reboot",
					Name:           "Reboot",
					RequiresTarget: true,
					SupportsDryRun: true,
				}},
			}

			sut.persistCanonicalConnectorSnapshot(tc.providerID, "collector-1", connector.DisplayName(), "", connector, tc.discovered)

			providerInstanceID := canonicalProviderInstanceID(model.ProviderKindConnector, tc.providerID, "collector-1")
			if _, ok, err := sut.canonicalStore.GetProviderInstance(providerInstanceID); err != nil {
				t.Fatalf("get provider instance: %v", err)
			} else if !ok {
				t.Fatalf("expected provider instance %q", providerInstanceID)
			}

			for _, asset := range tc.discovered {
				expected, hasExpected := expectedByID[asset.ID]
				if !hasExpected {
					t.Fatalf("missing expected canonical contract for asset %s", asset.ID)
				}

				storedAsset, ok, err := sut.assetStore.GetAsset(asset.ID)
				if err != nil {
					t.Fatalf("get asset %s: %v", asset.ID, err)
				}
				if !ok {
					t.Fatalf("expected persisted asset %s", asset.ID)
				}
				if storedAsset.ResourceClass != expected.class {
					t.Fatalf("asset %s resource_class = %q, want %q", asset.ID, storedAsset.ResourceClass, expected.class)
				}
				if storedAsset.ResourceKind != expected.kind {
					t.Fatalf("asset %s resource_kind = %q, want %q", asset.ID, storedAsset.ResourceKind, expected.kind)
				}
				if storedAsset.Metadata["resource_class"] != expected.class {
					t.Fatalf("asset %s metadata.resource_class = %q, want %q", asset.ID, storedAsset.Metadata["resource_class"], expected.class)
				}
				if storedAsset.Metadata["resource_kind"] != expected.kind {
					t.Fatalf("asset %s metadata.resource_kind = %q, want %q", asset.ID, storedAsset.Metadata["resource_kind"], expected.kind)
				}

				refs, err := sut.canonicalStore.ListResourceExternalRefs(asset.ID)
				if err != nil {
					t.Fatalf("list external refs for %s: %v", asset.ID, err)
				}
				if len(refs) == 0 {
					t.Fatalf("expected external refs for %s", asset.ID)
				}
				ref := refs[0]
				if ref.ProviderInstanceID != providerInstanceID {
					t.Fatalf("asset %s external ref provider_instance_id = %q, want %q", asset.ID, ref.ProviderInstanceID, providerInstanceID)
				}
				if strings.TrimSpace(ref.ExternalID) == "" {
					t.Fatalf("asset %s external ref external_id should not be empty", asset.ID)
				}
				if ref.ExternalType != expected.kind {
					t.Fatalf("asset %s external ref external_type = %q, want %q", asset.ID, ref.ExternalType, expected.kind)
				}
				binding, ok, err := sut.canonicalStore.GetTemplateBinding(asset.ID)
				if err != nil {
					t.Fatalf("get template binding for %s: %v", asset.ID, err)
				}
				if !ok {
					t.Fatalf("expected template binding for %s", asset.ID)
				}
				if len(binding.Tabs) == 0 {
					t.Fatalf("expected non-empty tabs for %s", asset.ID)
				}
			}

			if _, ok, err := sut.canonicalStore.GetCapabilitySet("provider", providerInstanceID); err != nil {
				t.Fatalf("get provider capability set: %v", err)
			} else if !ok {
				t.Fatalf("expected provider capability set for %s", providerInstanceID)
			}
			if _, ok, err := sut.canonicalStore.GetCapabilitySet("resource", tc.expectedResourceSetID); err != nil {
				t.Fatalf("get resource capability set: %v", err)
			} else if !ok {
				t.Fatalf("expected resource capability set for %s", tc.expectedResourceSetID)
			}

			if _, ok, err := sut.canonicalStore.GetIngestCheckpoint(providerInstanceID, "discover"); err != nil {
				t.Fatalf("get ingest checkpoint: %v", err)
			} else if !ok {
				t.Fatalf("expected discover checkpoint for %s", providerInstanceID)
			}

			reconciliations, err := sut.canonicalStore.ListReconciliationResults(providerInstanceID, 10)
			if err != nil {
				t.Fatalf("list reconciliation results: %v", err)
			}
			if len(reconciliations) == 0 {
				t.Fatalf("expected reconciliation result for %s", providerInstanceID)
			}

			relationships, err := sut.canonicalStore.ListResourceRelationships("", 200)
			if err != nil {
				t.Fatalf("list relationships: %v", err)
			}
			if tc.expectRelationships && len(relationships) == 0 {
				t.Fatalf("expected relationships for connector %s", tc.providerID)
			}

			snapshot := collectCanonicalContractSnapshot(t, sut, providerInstanceID, tc.discovered)
			assertCanonicalContractSnapshotFixture(t, tc.providerID, snapshot)
		})
	}
}

func TestHandleConnectorActionsDiscoverPersistsCanonicalSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("generic connector discover path", func(t *testing.T) {
		t.Parallel()

		sut := newTestAPIServer(t)
		discovered := []connectorsdk.Asset{
			{ID: "proxmox-node-pve99", Type: "hypervisor-node", Name: "pve99", Source: "proxmox", Metadata: map[string]string{"node": "pve99"}},
			{ID: "proxmox-vm-999", Type: "vm", Name: "vm-999", Source: "proxmox", Metadata: map[string]string{"node": "pve99", "vmid": "999"}},
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
			actions: []connectorsdk.ActionDescriptor{{ID: "vm.start", CanonicalID: "workload.start", Name: "Start VM", RequiresTarget: true, SupportsDryRun: true}},
			assets:  discovered,
		}
		sut.connectorRegistry.Register(connector)

		req := httptest.NewRequest(http.MethodGet, "/connectors/proxmox/discover", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		providerInstanceID := canonicalProviderInstanceID(model.ProviderKindConnector, "proxmox", "")
		if _, ok, err := sut.canonicalStore.GetProviderInstance(providerInstanceID); err != nil {
			t.Fatalf("get provider instance: %v", err)
		} else if !ok {
			t.Fatalf("expected provider instance %s", providerInstanceID)
		}
		if _, ok, err := sut.canonicalStore.GetIngestCheckpoint(providerInstanceID, "discover"); err != nil {
			t.Fatalf("get ingest checkpoint: %v", err)
		} else if !ok {
			t.Fatalf("expected discover checkpoint for %s", providerInstanceID)
		}
		if _, ok, err := sut.canonicalStore.GetTemplateBinding("proxmox-vm-999"); err != nil {
			t.Fatalf("get template binding: %v", err)
		} else if !ok {
			t.Fatalf("expected template binding for proxmox-vm-999")
		}

		snapshot := collectCanonicalContractSnapshot(t, sut, providerInstanceID, discovered)
		assertCanonicalDiscoverContractSnapshotFixture(t, "proxmox-discover", snapshot)
	})

	t.Run("portainer discover path", func(t *testing.T) {
		t.Parallel()

		sut := newTestAPIServer(t)
		discovered := []connectorsdk.Asset{
			{ID: "portainer-endpoint-main", Type: "container-host", Name: "main-endpoint", Source: "portainer", Metadata: map[string]string{"endpoint_id": "42"}},
			{ID: "portainer-ct-main", Type: "container", Name: "main-api", Source: "portainer", Metadata: map[string]string{"endpoint_id": "42"}},
		}
		for _, asset := range discovered {
			seedCanonicalContractAsset(t, sut, asset)
		}

		connector := &canonicalContractConnector{
			id:      "portainer",
			display: "Portainer",
			caps: connectorsdk.Capabilities{
				DiscoverAssets: true,
				CollectMetrics: true,
				CollectEvents:  true,
				ExecuteActions: true,
			},
			actions: []connectorsdk.ActionDescriptor{{ID: "container.restart", CanonicalID: "container.restart", Name: "Restart", RequiresTarget: true, SupportsDryRun: true}},
			assets:  discovered,
		}
		sut.connectorRegistry.Register(connector)

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/discover", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}

		providerInstanceID := canonicalProviderInstanceID(model.ProviderKindConnector, "portainer", "")
		if _, ok, err := sut.canonicalStore.GetProviderInstance(providerInstanceID); err != nil {
			t.Fatalf("get provider instance: %v", err)
		} else if !ok {
			t.Fatalf("expected provider instance %s", providerInstanceID)
		}
		if _, ok, err := sut.canonicalStore.GetCapabilitySet("provider", providerInstanceID); err != nil {
			t.Fatalf("get provider capability set: %v", err)
		} else if !ok {
			t.Fatalf("expected provider capability set for %s", providerInstanceID)
		}
		rels, err := sut.canonicalStore.ListResourceRelationships("portainer-ct-main", 20)
		if err != nil {
			t.Fatalf("list relationships: %v", err)
		}
		if len(rels) == 0 {
			t.Fatalf("expected relationship writes for portainer discover path")
		}

		snapshot := collectCanonicalContractSnapshot(t, sut, providerInstanceID, discovered)
		assertCanonicalDiscoverContractSnapshotFixture(t, "portainer-discover", snapshot)
	})
}
