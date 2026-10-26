package actionspkg

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	proxmoxapi "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

type effectiveMaintenanceWindows struct {
	persistence.GroupMaintenanceStore
	blocked bool
}

func (s *effectiveMaintenanceWindows) ListGroupMaintenanceWindows(group string, _ *time.Time, _ int) ([]groupmaintenance.MaintenanceWindow, error) {
	if s.blocked && group == "protected" {
		return []groupmaintenance.MaintenanceWindow{{ID: "window", GroupID: group, BlockActions: true}}, nil
	}
	return nil, nil
}

func TestConnectorActionMaintenanceUsesEffectiveVMBeforeQueueAndDispatch(t *testing.T) {
	for _, raw := range []bool{false, true} {
		name := "explicit override"
		if raw {
			name = "raw VM ID"
		}
		t.Run(name, func(t *testing.T) {
			deps, queue := connectorActionFixture(t, assets.Asset{ID: "truenas-vm-open", Source: "truenas", Type: "vm", Metadata: map[string]string{"vm_id": "42"}})
			_, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "truenas-vm-protected", Source: "truenas", Type: "vm", GroupID: "protected", Metadata: map[string]string{"vm_id": "43"}})
			if err != nil {
				t.Fatal(err)
			}
			windows := &effectiveMaintenanceWindows{blocked: true}
			groups := &groupfeatures.Deps{AssetStore: deps.AssetStore, GroupMaintenanceStore: windows}
			evaluate := (maintenanceguard.ConnectorEvaluator{Assets: deps.AssetStore, EvaluateAsset: groups.EvaluateAssetGuardrails}).Evaluate
			deps.EvaluateConnectorGuardrails = evaluate
			deps.EvaluateGuardrails, deps.ResolveGroupIDForAction = groups.EvaluateGuardrails, groups.ResolveGroupIDForAction
			req := actions.ExecuteRequest{ConnectorID: "truenas", ActionID: "vm.stop", Target: "truenas-vm-open", Params: map[string]string{"vm_id": "43"}}
			if raw {
				req.Target, req.Params = "43", nil
			}
			recorder := executeConnectorRequest(t, deps, req, nil)
			runs, listErr := deps.ActionStore.ListActionRuns(10, 0, "", "")
			if recorder.Code != http.StatusLocked || len(queue.jobs) != 0 || listErr != nil || len(runs) != 0 {
				t.Fatalf("locked request reached queue: status=%d jobs=%d runs=%d err=%v body=%s", recorder.Code, len(queue.jobs), len(runs), listErr, recorder.Body.String())
			}
			windows.blocked = false
			recorder = executeConnectorRequest(t, deps, req, nil)
			if recorder.Code != http.StatusAccepted || len(queue.jobs) != 1 {
				t.Fatalf("unlocked request denied: status=%d jobs=%d body=%s", recorder.Code, len(queue.jobs), recorder.Body.String())
			}
			job := queue.jobs[0]
			if job.Target != req.Target || job.Params["vm_id"] != req.Params["vm_id"] {
				t.Fatalf("provider/audit target changed: %+v", job)
			}
			providerCalls := 0
			worker := &proxmoxapi.Deps{AssetStore: deps.AssetStore, EvaluateConnectorGuardrails: evaluate,
				ExecuteActionInProcessFn: func(got actions.Job, _ *connectorsdk.Registry) actions.Result {
					providerCalls++
					if got.Target != req.Target || got.Params["vm_id"] != req.Params["vm_id"] {
						t.Errorf("worker changed provider request: %+v", got)
					}
					return actions.Result{Status: actions.StatusSucceeded}
				}}
			windows.blocked = true // The window began after the request was queued.
			result := worker.ExecuteActionInProcess(job)
			if result.Status != actions.StatusFailed || providerCalls != 0 || !strings.Contains(result.Error, "maintenance") {
				t.Fatalf("queued job bypassed new window: result=%+v calls=%d", result, providerCalls)
			}
			windows.blocked = false
			result = worker.ExecuteActionInProcess(job)
			if result.Status != actions.StatusSucceeded || providerCalls != 1 {
				t.Fatalf("unlocked queued job failed: result=%+v calls=%d", result, providerCalls)
			}
		})
	}
}

type effectiveProxmoxCollector struct{ persistence.HubCollectorStore }

func (effectiveProxmoxCollector) ListHubCollectors(int, bool) ([]hubcollector.Collector, error) {
	return []hubcollector.Collector{{ID: "pve-a", CollectorType: "proxmox", Enabled: true}}, nil
}

func (effectiveProxmoxCollector) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	return hubcollector.Collector{ID: "pve-a", CollectorType: "proxmox", Enabled: true}, id == "pve-a", nil
}

func TestConnectorActionProxmoxWorkerChecksRawTargetBeforeRuntime(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	_, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "proxmox-vm-100", Source: "proxmox", Type: "vm", GroupID: "protected", Metadata: map[string]string{"collector_id": "pve-a", "node": "node-a", "vmid": "100"}})
	if err != nil {
		t.Fatal(err)
	}
	groups := &groupfeatures.Deps{AssetStore: store, GroupMaintenanceStore: &effectiveMaintenanceWindows{blocked: true}}
	worker := &proxmoxapi.Deps{AssetStore: store, HubCollectorStore: effectiveProxmoxCollector{}}
	worker.EvaluateConnectorGuardrails = (maintenanceguard.ConnectorEvaluator{
		Assets: store, Collectors: worker.HubCollectorStore, EvaluateAsset: groups.EvaluateAssetGuardrails,
		ResolveProxmoxDestination: worker.ResolveProxmoxActionDestination,
	}).Evaluate
	// No credentials/runtime exist: a dispatch attempt would fail for that
	// reason instead. The maintenance owner must stop before loading either.
	result := worker.ExecuteActionInProcess(actions.Job{Type: actions.RunTypeConnectorAction, ConnectorID: "proxmox", ActionID: "vm.stop", Target: "node-a/100", Params: map[string]string{"collector_id": "pve-a"}})
	if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "maintenance") {
		t.Fatalf("raw worker target missed maintenance before runtime: %+v", result)
	}
}

func TestConnectorActionProxmoxAliasesAndNumericTargetsReachWorker(t *testing.T) {
	for _, tc := range []struct{ name, action, target string }{
		{"canonical alias", "workload.stop", "proxmox-vm-100"},
		{"zero-padded raw ID", "vm.stop", "node-a/0100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps, queue := connectorActionFixture(t, assets.Asset{ID: "proxmox-vm-100", Source: "proxmox", Type: "vm", Metadata: map[string]string{"collector_id": "pve-a", "node": "node-a", "vmid": "100"}})
			deps.ResolveProxmoxActionCollector = func(action, target string, params map[string]string) (string, error) {
				_, _, id, err := (&proxmoxapi.Deps{AssetStore: deps.AssetStore, HubCollectorStore: effectiveProxmoxCollector{}}).ResolveProxmoxActionDestination(action, target, params)
				return id, err
			}
			recorder := executeConnectorRequest(t, deps, actions.ExecuteRequest{ConnectorID: "proxmox", ActionID: tc.action, Target: tc.target, DryRun: true}, nil)
			if recorder.Code != http.StatusAccepted || len(queue.jobs) != 1 {
				t.Fatalf("status=%d jobs=%d body=%s", recorder.Code, len(queue.jobs), recorder.Body.String())
			}
			worker := &proxmoxapi.Deps{AssetStore: deps.AssetStore, HubCollectorStore: effectiveProxmoxCollector{}}
			result := worker.ExecuteActionInProcess(queue.jobs[0])
			if result.Status != actions.StatusSucceeded || result.Output != "would execute vm.stop on node-a/100" {
				t.Fatalf("wrong actual worker destination: %+v", result)
			}
		})
	}
}

type changingActionCollectors struct {
	persistence.HubCollectorStore
	selected string
}

func (s *changingActionCollectors) ListHubCollectors(int, bool) ([]hubcollector.Collector, error) {
	return []hubcollector.Collector{{ID: s.selected, CollectorType: "proxmox", Enabled: true}}, nil
}
func (s *changingActionCollectors) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	return hubcollector.Collector{ID: s.selected, CollectorType: "proxmox", Enabled: true}, id == s.selected, nil
}

func TestConnectorActionProxmoxQueuedCollectorCannotChange(t *testing.T) {
	for _, canonical := range []bool{false, true} {
		name, target := "raw singleton replacement", "node-a/100"
		if canonical {
			name, target = "canonical owner reassignment", "proxmox-vm-100"
		}
		t.Run(name, func(t *testing.T) {
			asset := assets.Asset{ID: "proxmox-vm-100", Source: "proxmox", Type: "vm", Metadata: map[string]string{"collector_id": "pve-a", "node": "node-a", "vmid": "100"}}
			deps, queue := connectorActionFixture(t, asset)
			collectors := &changingActionCollectors{selected: "pve-a"}
			worker := &proxmoxapi.Deps{AssetStore: deps.AssetStore, HubCollectorStore: collectors}
			evaluate := (maintenanceguard.ConnectorEvaluator{Assets: deps.AssetStore, Collectors: collectors, ResolveProxmoxDestination: worker.ResolveProxmoxActionDestination}).Evaluate
			worker.EvaluateConnectorGuardrails, deps.EvaluateConnectorGuardrails = evaluate, evaluate
			deps.ResolveProxmoxActionCollector = func(action, target string, params map[string]string) (string, error) {
				_, _, id, err := worker.ResolveProxmoxActionDestination(action, target, params)
				return id, err
			}
			recorder := executeConnectorRequest(t, deps, actions.ExecuteRequest{ConnectorID: "proxmox", ActionID: "vm.stop", Target: target, DryRun: true}, nil)
			if recorder.Code != http.StatusAccepted || len(queue.jobs) != 1 || queue.jobs[0].Params["collector_id"] != "pve-a" {
				t.Fatalf("owner not saved: status=%d jobs=%+v body=%s", recorder.Code, queue.jobs, recorder.Body.String())
			}
			if result := worker.ExecuteActionInProcess(queue.jobs[0]); result.Status != actions.StatusSucceeded {
				t.Fatalf("valid owner rejected: %+v", result)
			}
			collectors.selected = "pve-b"
			if canonical {
				asset.Metadata["collector_id"] = "pve-b"
				if _, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: asset.ID, Source: asset.Source, Type: asset.Type, Metadata: asset.Metadata}); err != nil {
					t.Fatal(err)
				}
			}
			result := worker.ExecuteActionInProcess(queue.jobs[0])
			if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "collector") {
				t.Fatalf("queued job switched provider: %+v", result)
			}
		})
	}
}
