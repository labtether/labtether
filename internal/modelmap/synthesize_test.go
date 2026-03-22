package modelmap

import (
	"context"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/model"
)

type synthConnector struct {
	id      string
	caps    connectorsdk.Capabilities
	actions []connectorsdk.ActionDescriptor
}

func (c *synthConnector) ID() string { return c.id }

func (c *synthConnector) DisplayName() string { return c.id }

func (c *synthConnector) Capabilities() connectorsdk.Capabilities { return c.caps }

func (c *synthConnector) Discover(context.Context) ([]connectorsdk.Asset, error) { return nil, nil }

func (c *synthConnector) TestConnection(context.Context) (connectorsdk.Health, error) {
	return connectorsdk.Health{Status: "ok", Message: "ok"}, nil
}

func (c *synthConnector) Actions() []connectorsdk.ActionDescriptor { return c.actions }

func (c *synthConnector) ExecuteAction(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	return connectorsdk.ActionResult{Status: "succeeded", Message: "ok"}, nil
}

func TestSynthesizeResourceRelationshipsProxmox(t *testing.T) {
	t.Parallel()

	rels := synthesizeResourceRelationshipsAt("proxmox", []connectorsdk.Asset{
		{ID: "proxmox-node-pve01", Type: "hypervisor-node", Name: "pve01", Source: "proxmox"},
		{ID: "proxmox-vm-100", Type: "vm", Name: "vm-100", Source: "proxmox", Metadata: map[string]string{"node": "pve01"}},
		{ID: "proxmox-ct-101", Type: "container", Name: "ct-101", Source: "proxmox", Metadata: map[string]string{"node": "pve01"}},
	}, time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC))

	if len(rels) != 2 {
		t.Fatalf("len(rels) = %d, want 2", len(rels))
	}
	assertHasRelationship(t, rels, "proxmox-vm-100", "proxmox-node-pve01", model.RelationshipRunsOn)
	assertHasRelationship(t, rels, "proxmox-ct-101", "proxmox-node-pve01", model.RelationshipRunsOn)
}

func TestSynthesizeResourceRelationshipsPortainer(t *testing.T) {
	t.Parallel()

	rels := synthesizeResourceRelationshipsAt("portainer", []connectorsdk.Asset{
		{ID: "portainer-endpoint-1", Type: "container-host", Name: "endpoint-1", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1"}},
		{ID: "portainer-stack-1", Type: "stack", Name: "infra", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1", "stack_id": "1"}},
		{ID: "portainer-container-1-abcd", Type: "container", Name: "nginx", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1", "stack": "infra", "container_id": "abcd"}},
	}, time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC))

	if len(rels) != 3 {
		t.Fatalf("len(rels) = %d, want 3", len(rels))
	}
	assertHasRelationship(t, rels, "portainer-container-1-abcd", "portainer-endpoint-1", model.RelationshipRunsOn)
	assertHasRelationship(t, rels, "portainer-stack-1", "portainer-endpoint-1", model.RelationshipRunsOn)
	assertHasRelationship(t, rels, "portainer-container-1-abcd", "portainer-stack-1", model.RelationshipMemberOf)
}

func TestSynthesizeResourceRelationshipsPBS(t *testing.T) {
	t.Parallel()

	rels := synthesizeResourceRelationshipsAt("pbs", []connectorsdk.Asset{
		{ID: "pbs-server-lab", Type: "storage-controller", Name: "pbs-lab", Source: "pbs"},
		{ID: "pbs-datastore-backup", Type: "storage-pool", Name: "backup", Source: "pbs", Metadata: map[string]string{"store": "backup"}},
	}, time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC))

	if len(rels) != 1 {
		t.Fatalf("len(rels) = %d, want 1", len(rels))
	}
	assertHasRelationship(t, rels, "pbs-server-lab", "pbs-datastore-backup", model.RelationshipContains)
}

func TestSynthesizeCapabilitySets(t *testing.T) {
	t.Parallel()

	connector := &synthConnector{
		id: "proxmox",
		caps: connectorsdk.Capabilities{
			DiscoverAssets: true,
			CollectMetrics: true,
			CollectEvents:  true,
			ExecuteActions: true,
		},
		actions: []connectorsdk.ActionDescriptor{
			{ID: "vm.start", Name: "Start VM", RequiresTarget: true, SupportsDryRun: true},
			{ID: "pool.scrub", Name: "Scrub Pool", RequiresTarget: true, SupportsDryRun: true},
			{ID: "system.reboot", Name: "Reboot", RequiresTarget: true},
		},
	}

	sets := synthesizeCapabilitySetsAt(connector, []connectorsdk.Asset{
		{ID: "proxmox-vm-100", Type: "vm", Name: "vm-100", Source: "proxmox"},
		{ID: "proxmox-storage-local", Type: "storage-pool", Name: "local", Source: "proxmox"},
	}, time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC))

	if len(sets) != 3 {
		t.Fatalf("len(sets) = %d, want 3", len(sets))
	}

	provider := capabilitySetBySubject(sets, "provider", "proxmox")
	if provider == nil {
		t.Fatalf("expected provider capability set")
	}
	assertHasCapability(t, provider.Capabilities, "inventory.discover")
	assertHasCapability(t, provider.Capabilities, "workload.action")
	assertHasCapability(t, provider.Capabilities, "system.action")

	vmSet := capabilitySetBySubject(sets, "resource", "proxmox-vm-100")
	if vmSet == nil {
		t.Fatalf("expected vm capability set")
	}
	assertHasCapability(t, vmSet.Capabilities, "workload.action")

	storageSet := capabilitySetBySubject(sets, "resource", "proxmox-storage-local")
	if storageSet == nil {
		t.Fatalf("expected storage capability set")
	}
	assertHasCapability(t, storageSet.Capabilities, "system.action")
}

func TestConnectorCanonicalConformanceTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		source    string
		assetType string
		metadata  map[string]string
		wantKind  string
		wantClass string
	}{
		{source: "proxmox", assetType: "hypervisor-node", metadata: map[string]string{"node": "pve01"}, wantKind: "hypervisor-node", wantClass: "compute"},
		{source: "pbs", assetType: "storage-pool", metadata: map[string]string{"store": "backup"}, wantKind: "datastore", wantClass: "storage"},
		{source: "portainer", assetType: "container", metadata: map[string]string{"container_id": "abc"}, wantKind: "container", wantClass: "compute"},
		{source: "truenas", assetType: "nas", metadata: map[string]string{"hostname": "omega"}, wantKind: "storage-controller", wantClass: "storage"},
		{source: "home-assistant", assetType: "ha-entity", metadata: map[string]string{"entity_id": "sensor.temp"}, wantKind: "ha-entity", wantClass: "service"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.source+"/"+tc.assetType, func(t *testing.T) {
			t.Parallel()

			asset := CanonicalizeConnectorAsset(tc.source, connectorsdk.Asset{
				ID:       tc.source + "-asset",
				Type:     tc.assetType,
				Name:     "asset",
				Source:   tc.source,
				Metadata: tc.metadata,
			})
			if asset.Kind != tc.wantKind {
				t.Fatalf("asset.Kind = %q, want %q", asset.Kind, tc.wantKind)
			}
			if asset.Class != tc.wantClass {
				t.Fatalf("asset.Class = %q, want %q", asset.Class, tc.wantClass)
			}
			if asset.Attributes == nil {
				t.Fatalf("expected canonical attributes")
			}
		})
	}
}

func assertHasRelationship(t *testing.T, rels []model.ResourceRelationship, source, target string, relType model.RelationshipType) {
	t.Helper()
	for _, rel := range rels {
		if rel.SourceResourceID == source && rel.TargetResourceID == target && rel.Type == relType {
			return
		}
	}
	t.Fatalf("missing relationship %s -> %s (%s)", source, target, relType)
}

func capabilitySetBySubject(sets []model.CapabilitySet, subjectType, subjectID string) *model.CapabilitySet {
	for idx := range sets {
		if sets[idx].SubjectType == subjectType && sets[idx].SubjectID == subjectID {
			return &sets[idx]
		}
	}
	return nil
}

func assertHasCapability(t *testing.T, specs []model.CapabilitySpec, id string) {
	t.Helper()
	for _, spec := range specs {
		if spec.ID == id {
			return
		}
	}
	t.Fatalf("missing capability %q", id)
}
