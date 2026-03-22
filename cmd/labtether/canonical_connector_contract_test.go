package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/model"
)

var updateCanonicalContractFixtures = flag.Bool(
	"update-canonical-contract-fixtures",
	false,
	"update canonical connector contract fixture baselines",
)

type canonicalContractConnector struct {
	id      string
	display string
	caps    connectorsdk.Capabilities
	actions []connectorsdk.ActionDescriptor
	assets  []connectorsdk.Asset
}

func (c *canonicalContractConnector) ID() string { return c.id }

func (c *canonicalContractConnector) DisplayName() string {
	if strings.TrimSpace(c.display) != "" {
		return c.display
	}
	return c.id
}

func (c *canonicalContractConnector) Capabilities() connectorsdk.Capabilities { return c.caps }

func (c *canonicalContractConnector) Discover(context.Context) ([]connectorsdk.Asset, error) {
	out := make([]connectorsdk.Asset, len(c.assets))
	copy(out, c.assets)
	return out, nil
}

func (c *canonicalContractConnector) TestConnection(context.Context) (connectorsdk.Health, error) {
	return connectorsdk.Health{Status: "ok", Message: "ok"}, nil
}

func (c *canonicalContractConnector) Actions() []connectorsdk.ActionDescriptor {
	out := make([]connectorsdk.ActionDescriptor, len(c.actions))
	copy(out, c.actions)
	return out
}

func (c *canonicalContractConnector) ExecuteAction(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	return connectorsdk.ActionResult{Status: "ok", Message: "ok"}, nil
}

type canonicalContractSnapshot struct {
	ProviderInstance canonicalContractProviderInstanceFixture  `json:"provider_instance"`
	Assets           []canonicalContractAssetFixture           `json:"assets,omitempty"`
	ExternalRefs     []canonicalContractExternalRefFixture     `json:"external_refs,omitempty"`
	Relationships    []canonicalContractRelationshipFixture    `json:"relationships,omitempty"`
	CapabilitySets   []canonicalContractCapabilitySetFixture   `json:"capability_sets,omitempty"`
	TemplateBindings []canonicalContractTemplateBindingFixture `json:"template_bindings,omitempty"`
	Checkpoint       canonicalContractCheckpointFixture        `json:"checkpoint"`
	Reconciliation   canonicalContractReconciliationFixture    `json:"reconciliation"`
}

type canonicalContractProviderInstanceFixture struct {
	ID          string               `json:"id"`
	Kind        model.ProviderKind   `json:"kind"`
	Provider    string               `json:"provider"`
	DisplayName string               `json:"display_name"`
	Status      model.ProviderStatus `json:"status"`
	Scope       model.ProviderScope  `json:"scope"`
	ConfigRef   string               `json:"config_ref,omitempty"`
	Metadata    map[string]any       `json:"metadata,omitempty"`
}

type canonicalContractAssetFixture struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	Name          string            `json:"name"`
	Source        string            `json:"source"`
	ResourceClass string            `json:"resource_class"`
	ResourceKind  string            `json:"resource_kind"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Attributes    map[string]any    `json:"attributes,omitempty"`
}

type canonicalContractExternalRefFixture struct {
	ResourceID         string `json:"resource_id"`
	ProviderInstanceID string `json:"provider_instance_id"`
	ExternalID         string `json:"external_id"`
	ExternalType       string `json:"external_type,omitempty"`
	ExternalParentID   string `json:"external_parent_id,omitempty"`
	RawLocator         string `json:"raw_locator,omitempty"`
}

type canonicalContractRelationshipFixture struct {
	SourceResourceID string                        `json:"source_resource_id"`
	TargetResourceID string                        `json:"target_resource_id"`
	Type             model.RelationshipType        `json:"type"`
	Direction        model.RelationshipDirection   `json:"direction"`
	Criticality      model.RelationshipCriticality `json:"criticality"`
	Inferred         bool                          `json:"inferred"`
	Confidence       int                           `json:"confidence"`
	Evidence         map[string]any                `json:"evidence,omitempty"`
}

type canonicalContractCapabilityFixture struct {
	ID             string                    `json:"id"`
	Scope          model.CapabilityScope     `json:"scope"`
	Stability      model.CapabilityStability `json:"stability,omitempty"`
	SupportsDryRun bool                      `json:"supports_dry_run,omitempty"`
	SupportsAsync  bool                      `json:"supports_async,omitempty"`
	RequiresTarget bool                      `json:"requires_target,omitempty"`
}

type canonicalContractCapabilitySetFixture struct {
	SubjectType  string                               `json:"subject_type"`
	SubjectID    string                               `json:"subject_id"`
	Capabilities []canonicalContractCapabilityFixture `json:"capabilities,omitempty"`
}

type canonicalContractTemplateBindingFixture struct {
	ResourceID string   `json:"resource_id"`
	TemplateID string   `json:"template_id"`
	Tabs       []string `json:"tabs,omitempty"`
	Operations []string `json:"operations,omitempty"`
}

type canonicalContractCheckpointFixture struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	Stream             string `json:"stream"`
	Cursor             string `json:"cursor"`
}

type canonicalContractReconciliationFixture struct {
	CreatedCount int `json:"created_count"`
	UpdatedCount int `json:"updated_count"`
	StaleCount   int `json:"stale_count"`
	ErrorCount   int `json:"error_count"`
}

type canonicalHeartbeatSnapshot struct {
	ProviderInstance canonicalContractProviderInstanceFixture `json:"provider_instance"`
	Asset            canonicalContractAssetFixture            `json:"asset"`
	ExternalRefs     []canonicalContractExternalRefFixture    `json:"external_refs,omitempty"`
	CapabilitySet    canonicalContractCapabilitySetFixture    `json:"capability_set"`
	TemplateBinding  canonicalContractTemplateBindingFixture  `json:"template_binding"`
	Checkpoint       canonicalContractCheckpointFixture       `json:"checkpoint"`
}

type canonicalStatusAggregateSnapshot struct {
	Registry         canonicalStatusRegistryFixture             `json:"registry"`
	Providers        []canonicalContractProviderInstanceFixture `json:"providers,omitempty"`
	CapabilitySets   []canonicalContractCapabilitySetFixture    `json:"capability_sets,omitempty"`
	TemplateBindings []canonicalContractTemplateBindingFixture  `json:"template_bindings,omitempty"`
	Reconciliation   []canonicalStatusReconciliationFixture     `json:"reconciliation,omitempty"`
}

type canonicalStatusRegistryFixture struct {
	CapabilityIDs []string `json:"capability_ids,omitempty"`
	OperationIDs  []string `json:"operation_ids,omitempty"`
	MetricIDs     []string `json:"metric_ids,omitempty"`
	EventIDs      []string `json:"event_ids,omitempty"`
	TemplateIDs   []string `json:"template_ids,omitempty"`
}

type canonicalStatusReconciliationFixture struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	CreatedCount       int    `json:"created_count"`
	UpdatedCount       int    `json:"updated_count"`
	StaleCount         int    `json:"stale_count"`
	ErrorCount         int    `json:"error_count"`
}

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

func seedCanonicalContractAsset(t *testing.T, sut *apiServer, asset connectorsdk.Asset) {
	t.Helper()

	_, metadata := withCanonicalResourceMetadata(asset.Source, asset.Type, asset.Metadata)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  asset.ID,
		Type:     asset.Type,
		Name:     asset.Name,
		Source:   asset.Source,
		Status:   "online",
		Metadata: metadata,
	}); err != nil {
		t.Fatalf("seed asset %s: %v", asset.ID, err)
	}
}

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

func assertCanonicalContractSnapshotFixture(t *testing.T, providerID string, actual canonicalContractSnapshot) {
	t.Helper()
	assertCanonicalContractSnapshotFixtureInDir(t, "canonical_connector_contract", providerID, actual)
}

func assertCanonicalDiscoverContractSnapshotFixture(t *testing.T, caseID string, actual canonicalContractSnapshot) {
	t.Helper()
	assertCanonicalContractSnapshotFixtureInDir(t, "canonical_discover_contract", caseID, actual)
}

func assertCanonicalContractSnapshotFixtureInDir(t *testing.T, fixtureDir, caseID string, actual canonicalContractSnapshot) {
	t.Helper()
	assertCanonicalFixture(t, fixtureDir, caseID, actual)
}

func assertCanonicalHeartbeatSnapshotFixture(t *testing.T, platform string, actual canonicalHeartbeatSnapshot) {
	t.Helper()
	assertCanonicalFixture(t, "canonical_heartbeat_contract", platform, actual)
}

func assertCanonicalStatusAggregateSnapshotFixture(t *testing.T, caseID string, actual canonicalStatusAggregateSnapshot) {
	t.Helper()
	assertCanonicalFixture(t, "canonical_status_aggregate_contract", caseID, actual)
}

func assertCanonicalFixture[T any](t *testing.T, fixtureDir, caseID string, actual T) {
	t.Helper()

	fixturePath := filepath.Join("testdata", strings.TrimSpace(fixtureDir), strings.TrimSpace(caseID)+".json")
	actualJSON, err := json.MarshalIndent(actual, "", "  ")
	if err != nil {
		t.Fatalf("marshal canonical snapshot for %s/%s: %v", fixtureDir, caseID, err)
	}
	actualJSON = append(actualJSON, '\n')

	if *updateCanonicalContractFixtures {
		if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
			t.Fatalf("create fixture directory for %s/%s: %v", fixtureDir, caseID, err)
		}
		if err := os.WriteFile(fixturePath, actualJSON, 0o644); err != nil {
			t.Fatalf("write fixture for %s/%s: %v", fixtureDir, caseID, err)
		}
	}

	expectedJSON, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture for %s/%s: %v", fixtureDir, caseID, err)
	}

	var expected T
	if err := json.Unmarshal(expectedJSON, &expected); err != nil {
		t.Fatalf("decode fixture for %s/%s: %v", fixtureDir, caseID, err)
	}

	if !reflect.DeepEqual(expected, actual) {
		expectedPretty, marshalErr := json.MarshalIndent(expected, "", "  ")
		if marshalErr != nil {
			t.Fatalf("marshal expected fixture for %s/%s: %v", fixtureDir, caseID, marshalErr)
		}
		t.Fatalf(
			"canonical fixture mismatch for %s/%s\nexpected:\n%s\nactual:\n%s",
			fixtureDir,
			caseID,
			string(expectedPretty),
			string(actualJSON),
		)
	}
}

func canonicalContractNormalizeCursor(cursor string) string {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return ""
	}
	if separator := strings.LastIndex(cursor, "@"); separator > 0 {
		prefix := strings.TrimSpace(cursor[:separator])
		if prefix == "" {
			return "@*"
		}
		return prefix + "@*"
	}
	segments := strings.Split(cursor, ";")
	for index, segment := range segments {
		trimmed := strings.TrimSpace(segment)
		if strings.HasPrefix(trimmed, "ts=") {
			segments[index] = "ts=*"
			continue
		}
		segments[index] = trimmed
	}
	return strings.Join(segments, ";")
}

func canonicalContractCloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func canonicalContractCloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func containsTab(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
