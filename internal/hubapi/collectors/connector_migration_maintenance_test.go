package collectors

import (
	"net/http"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

func TestDirectProxmoxPlacementHonorsDestinationMaintenance(t *testing.T) {
	for _, action := range []string{"vm.migrate", "ct.migrate", "vm.clone_from_template", "ct.clone_from_template"} {
		for _, tc := range []struct {
			name, sourceGroup, destinationGroup string
			foreignLegacy                       bool
			want                                int
		}{
			{name: "destination locked", destinationGroup: "locked", want: http.StatusLocked},
			{name: "normalized source locked", sourceGroup: "locked", want: http.StatusLocked},
			{name: "unlocked", want: http.StatusOK},
			{name: "foreign legacy destination locked", foreignLegacy: true, want: http.StatusOK},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				store := persistence.NewMemoryAssetStore()
				scoped := func(id string) string { return assetid.ScopeCollectorAssetID(id, "pve-b") }
				for _, asset := range []assets.Asset{
					{ID: scoped("proxmox-node-node-a"), Source: "proxmox", Type: "hypervisor-node", GroupID: tc.sourceGroup, Metadata: map[string]string{"collector_id": "pve-b", "node": "NODE-A"}},
					{ID: scoped("proxmox-node-node-b"), Source: "proxmox", Type: "hypervisor-node", GroupID: tc.destinationGroup, Metadata: map[string]string{"collector_id": "pve-b", "node": "NODE-B"}},
				} {
					storeEffectiveAsset(t, store, asset)
				}
				if tc.foreignLegacy {
					storeEffectiveAsset(t, store, assets.Asset{ID: "proxmox-node-node-b", Source: "proxmox", Type: "hypervisor-node", GroupID: "locked", Metadata: map[string]string{"collector_id": "pve-a", "node": "NODE-B"}})
				}
				provider := &directActionProvider{id: "proxmox", action: action}
				deps := effectiveActionDeps(store, provider, actionCollectorInventory{entries: []hubcollector.Collector{
					{ID: "pve-a", CollectorType: "proxmox", Enabled: true}, {ID: "pve-b", CollectorType: "proxmox", Enabled: true},
				}})
				req := connectorsdk.ActionRequest{TargetID: "NODE-A/100", Params: map[string]string{"collector_id": "pve-b", "target_node": " NODE-B ", "new_id": "101", "new_name": "clone"}}
				recorder := directActionRequest(t, deps, "proxmox", action, req, false)
				wantCalls := 0
				if tc.want == http.StatusOK {
					wantCalls = 1
				}
				if recorder.Code != tc.want || provider.calls != wantCalls {
					t.Fatalf("status=%d calls=%d body=%s", recorder.Code, provider.calls, recorder.Body.String())
				}
				if wantCalls == 1 && (provider.last.TargetID != req.TargetID || provider.last.Params["target_node"] != req.Params["target_node"] || provider.last.Params["collector_id"] != "pve-b") {
					t.Fatalf("changed migration routing: %+v", provider.last)
				}
			})
		}
	}
}
