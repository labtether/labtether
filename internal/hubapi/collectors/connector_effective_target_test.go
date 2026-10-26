package collectors

import (
	"errors"
	"net/http"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	proxmoxapi "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

type actionCollectorInventory struct {
	persistence.HubCollectorStore
	entries []hubcollector.Collector
}

func (s actionCollectorInventory) ListHubCollectors(int, bool) ([]hubcollector.Collector, error) {
	return s.entries, nil
}

func (s actionCollectorInventory) FindEnabledHubCollectorsByType(kind string) ([]hubcollector.Collector, error) {
	var found []hubcollector.Collector
	for _, entry := range s.entries {
		if entry.Enabled && hubcollector.NormalizeCollectorType(entry.CollectorType) == kind {
			found = append(found, entry)
			if len(found) == 2 {
				break
			}
		}
	}
	return found, nil
}

func (s actionCollectorInventory) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	for _, entry := range s.entries {
		if entry.ID == id {
			return entry, true, nil
		}
	}
	return hubcollector.Collector{}, false, nil
}

func storeEffectiveAsset(t *testing.T, store persistence.AssetStore, asset assets.Asset) {
	t.Helper()
	_, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: asset.ID, Name: asset.Name, Type: asset.Type, Source: asset.Source,
		GroupID: asset.GroupID, Metadata: asset.Metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func effectiveActionDeps(store persistence.AssetStore, provider *directActionProvider, inventory actionCollectorInventory) *Deps {
	groups := &groupfeatures.Deps{AssetStore: store, GroupMaintenanceStore: directActionMaintenanceStore{}}
	proxmox := &proxmoxapi.Deps{AssetStore: store, HubCollectorStore: inventory}
	registry := connectorsdk.NewRegistry()
	registry.Register(provider)
	evaluator := maintenanceguard.ConnectorEvaluator{
		Assets: store, Collectors: inventory, Registry: registry,
		EvaluateAsset: groups.EvaluateAssetGuardrails, ResolveProxmoxDestination: proxmox.ResolveProxmoxActionDestination,
	}
	return &Deps{AssetStore: store, HubCollectorStore: inventory, ConnectorRegistry: registry,
		EvaluateAssetGuardrails: groups.EvaluateAssetGuardrails, EvaluateConnectorGuardrails: evaluator.Evaluate,
		ExecuteProxmoxActionDirect: provider.ExecuteAction}
}

func TestDirectTrueNASMaintenanceUsesEffectiveVMAndHost(t *testing.T) {
	for _, tc := range []struct {
		name, vmGroup, nasGroup, parentGroup string
		raw, duplicate, foreign              bool
		want                                 int
	}{
		{name: "override locked VM", vmGroup: "locked", want: http.StatusLocked},
		{name: "raw locked VM", vmGroup: "locked", raw: true, want: http.StatusLocked},
		{name: "override open VM", want: http.StatusOK},
		{name: "raw open VM", raw: true, want: http.StatusOK},
		{name: "NAS host locked", nasGroup: "locked", want: http.StatusLocked},
		{name: "configured parent locked", parentGroup: "locked", want: http.StatusLocked},
		{name: "duplicate VM identity", duplicate: true, want: http.StatusConflict},
		{name: "same ID on other collector", foreign: true, want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := persistence.NewMemoryAssetStore()
			scoped := func(id string) string { return assetid.ScopeCollectorAssetID(id, "nas-a") }
			for _, asset := range []assets.Asset{
				{ID: scoped("truenas-vm-open"), Source: "truenas", Type: "vm", Metadata: map[string]string{"collector_id": "nas-a", "vm_id": "42"}},
				{ID: scoped("truenas-vm-effective"), Source: "truenas", Type: "vm", GroupID: tc.vmGroup, Metadata: map[string]string{"collector_id": "nas-a", "vm_id": "43"}},
				{ID: scoped("truenas-host-storage"), Source: "truenas", Type: "nas", GroupID: tc.nasGroup, Metadata: map[string]string{"collector_id": "nas-a"}},
				{ID: "configured-parent", Source: "agent", Type: "host", GroupID: tc.parentGroup},
			} {
				storeEffectiveAsset(t, store, asset)
			}
			if tc.duplicate {
				storeEffectiveAsset(t, store, assets.Asset{ID: scoped("truenas-vm-duplicate"), Source: "truenas", Type: "vm", Metadata: map[string]string{"collector_id": "nas-a", "vm_id": "43"}})
			}
			if tc.foreign {
				storeEffectiveAsset(t, store, assets.Asset{ID: assetid.ScopeCollectorAssetID("truenas-vm-foreign", "nas-b"), Source: "truenas", Type: "vm", GroupID: "locked", Metadata: map[string]string{"collector_id": "nas-b", "vm_id": "43"}})
			}
			provider := &directActionProvider{id: "truenas", action: "vm.stop"}
			deps := effectiveActionDeps(store, provider, actionCollectorInventory{entries: []hubcollector.Collector{{ID: "nas-a", CollectorType: "truenas", Enabled: true, AssetID: "configured-parent"}}})
			req := connectorsdk.ActionRequest{TargetID: scoped("truenas-vm-open"), Params: map[string]string{"vm_id": "43"}}
			if tc.raw {
				req.TargetID, req.Params = "43", nil
			}
			recorder := directActionRequest(t, deps, "truenas", "vm.stop", req, false)
			wantCalls := 0
			if tc.want == http.StatusOK {
				wantCalls = 1
			}
			if recorder.Code != tc.want || provider.calls != wantCalls {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, provider.calls, recorder.Body.String())
			}
			if wantCalls == 1 && (provider.last.TargetID != req.TargetID || provider.last.Params["vm_id"] != req.Params["vm_id"]) {
				t.Fatalf("changed valid provider request: %+v", provider.last)
			}
		})
	}
}

func TestDirectProxmoxMaintenanceUsesSelectedCollectorAndNode(t *testing.T) {
	for _, tc := range []struct {
		name, vmGroup, nodeGroup           string
		scoped, foreignLegacy, leadingZero bool
		want                               int
	}{
		{name: "raw locked VM", vmGroup: "locked", want: http.StatusLocked},
		{name: "zero-padded locked VM", vmGroup: "locked", leadingZero: true, want: http.StatusLocked},
		{name: "raw locked node", nodeGroup: "locked", want: http.StatusLocked},
		{name: "raw unlocked", want: http.StatusOK},
		{name: "scoped unlocked", scoped: true, want: http.StatusOK},
		{name: "other collector owns legacy VMID", foreignLegacy: true, want: http.StatusOK},
		{name: "scoped target ignores foreign legacy", foreignLegacy: true, scoped: true, want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := persistence.NewMemoryAssetStore()
			vm := assetid.ScopeCollectorAssetID("proxmox-vm-100", "pve-b")
			storeEffectiveAsset(t, store, assets.Asset{ID: vm, Source: "proxmox", Type: "vm", GroupID: tc.vmGroup, Metadata: map[string]string{"collector_id": "pve-b", "node": "node-b", "vmid": "100"}})
			storeEffectiveAsset(t, store, assets.Asset{ID: assetid.ScopeCollectorAssetID("proxmox-node-node-b", "pve-b"), Source: "proxmox", Type: "hypervisor-node", GroupID: tc.nodeGroup, Metadata: map[string]string{"collector_id": "pve-b", "node": "node-b"}})
			if tc.foreignLegacy {
				storeEffectiveAsset(t, store, assets.Asset{ID: "proxmox-vm-100", Source: "proxmox", Type: "vm", GroupID: "locked", Metadata: map[string]string{"collector_id": "pve-a", "node": "node-a", "vmid": "100"}})
				storeEffectiveAsset(t, store, assets.Asset{ID: "proxmox-node-node-b", Source: "proxmox", Type: "hypervisor-node", GroupID: "locked", Metadata: map[string]string{"collector_id": "pve-a", "node": "node-b"}})
			}
			provider := &directActionProvider{id: "proxmox", action: "vm.stop"}
			deps := effectiveActionDeps(store, provider, actionCollectorInventory{entries: []hubcollector.Collector{
				{ID: "pve-a", CollectorType: "proxmox", Enabled: true}, {ID: "pve-b", CollectorType: "proxmox", Enabled: true},
			}})
			req := connectorsdk.ActionRequest{TargetID: "node-b/100", Params: map[string]string{"collector_id": "pve-b"}}
			if tc.leadingZero {
				req.TargetID = "node-b/0100"
			}
			if tc.scoped {
				req.TargetID = vm
			}
			recorder := directActionRequest(t, deps, "proxmox", "vm.stop", req, false)
			wantCalls := 0
			if tc.want == http.StatusOK {
				wantCalls = 1
			}
			if recorder.Code != tc.want || provider.calls != wantCalls {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, provider.calls, recorder.Body.String())
			}
		})
	}
}

type unavailableTrueNASLookup struct{ *persistence.MemoryAssetStore }

func (unavailableTrueNASLookup) FindTrueNASActionAssets(string, string, string) ([]assets.Asset, error) {
	return nil, errors.New("inventory query failed")
}

func TestDirectConnectorEffectiveLookupFailureDoesNotDispatch(t *testing.T) {
	store := unavailableTrueNASLookup{persistence.NewMemoryAssetStore()}
	provider := &directActionProvider{id: "truenas", action: "vm.stop"}
	deps := effectiveActionDeps(store, provider, actionCollectorInventory{})
	recorder := directActionRequest(t, deps, "truenas", "vm.stop", connectorsdk.ActionRequest{TargetID: "43"}, false)
	if recorder.Code != http.StatusConflict || provider.calls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", recorder.Code, provider.calls, recorder.Body.String())
	}
}
