package actionspkg

import (
	"net/http"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	proxmoxapi "github.com/labtether/labtether/internal/hubapi/proxmox"
)

func TestConnectorPlacementChecksDestinationBeforeQueueAndExecution(t *testing.T) {
	for _, action := range []string{"vm.migrate", "ct.migrate", "vm.clone_from_template", "ct.clone_from_template"} {
		t.Run(action, func(t *testing.T) {
			deps, queue := connectorActionFixture(t, assets.Asset{ID: "proxmox-node-node-b", Source: "proxmox", Type: "hypervisor-node", GroupID: "protected", Metadata: map[string]string{"collector_id": "pve-a", "node": "node-b"}})
			windows := &effectiveMaintenanceWindows{blocked: true}
			groups := &groupfeatures.Deps{AssetStore: deps.AssetStore, GroupMaintenanceStore: windows}
			worker := &proxmoxapi.Deps{AssetStore: deps.AssetStore, HubCollectorStore: effectiveProxmoxCollector{}}
			evaluate := (maintenanceguard.ConnectorEvaluator{
				Assets: deps.AssetStore, Collectors: worker.HubCollectorStore,
				EvaluateAsset: groups.EvaluateAssetGuardrails, ResolveProxmoxDestination: worker.ResolveProxmoxActionDestination,
			}).Evaluate
			worker.EvaluateConnectorGuardrails, deps.EvaluateConnectorGuardrails = evaluate, evaluate
			deps.ResolveProxmoxActionCollector = func(action, target string, params map[string]string) (string, error) {
				_, _, collector, err := worker.ResolveProxmoxActionDestination(action, target, params)
				return collector, err
			}
			req := actions.ExecuteRequest{ConnectorID: "proxmox", ActionID: action, Target: "node-a/100", Params: map[string]string{"target_node": "node-b", "new_id": "101", "new_name": "clone"}}
			recorder := executeConnectorRequest(t, deps, req, nil)
			runs, err := deps.ActionStore.ListActionRuns(10, 0, "", "")
			if recorder.Code != http.StatusLocked || len(queue.jobs) != 0 || err != nil || len(runs) != 0 {
				t.Fatalf("locked destination reached queue: status=%d jobs=%d runs=%d err=%v", recorder.Code, len(queue.jobs), len(runs), err)
			}
			windows.blocked = false
			recorder = executeConnectorRequest(t, deps, req, nil)
			if recorder.Code != http.StatusAccepted || len(queue.jobs) != 1 {
				t.Fatalf("unlocked migration denied: status=%d jobs=%d body=%s", recorder.Code, len(queue.jobs), recorder.Body.String())
			}
			job := queue.jobs[0]
			if job.Target != req.Target || job.Params["target_node"] != "node-b" || job.Params["collector_id"] != "pve-a" {
				t.Fatalf("queued migration routing changed: %+v", job)
			}
			windows.blocked = true // The destination window starts after acceptance.
			result := worker.ExecuteActionInProcess(job)
			if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "maintenance") {
				t.Fatalf("destination window did not stop worker before provider runtime: %+v", result)
			}
		})
	}
}
