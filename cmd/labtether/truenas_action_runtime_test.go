package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/assetstatus"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
)

type trueNASActionWindows struct {
	persistence.GroupMaintenanceStore
	blocked string
}

func (s *trueNASActionWindows) ListGroupMaintenanceWindows(group string, _ *time.Time, _ int) ([]groupmaintenance.MaintenanceWindow, error) {
	if group == s.blocked && group != "" {
		return []groupmaintenance.MaintenanceWindow{{ID: "window", GroupID: group, BlockActions: true}}, nil
	}
	return nil, nil
}

type trueNASActionQueue struct{ jobs []actions.Job }

func (q *trueNASActionQueue) Enqueue(_ context.Context, _ jobqueue.JobKind, payload []byte) (string, error) {
	var job actions.Job
	if err := json.Unmarshal(payload, &job); err != nil {
		return "", err
	}
	q.jobs = append(q.jobs, job)
	return "queued", nil
}

type trueNASBindingDriftStore struct {
	persistence.AssetStore
	target  string
	reads   int
	change  func() error
	changed bool
}

func (s *trueNASBindingDriftStore) GetAsset(id string) (assets.Asset, bool, error) {
	if id == s.target {
		s.reads++
		if s.reads == 2 {
			if err := s.change(); err != nil {
				return assets.Asset{}, false, err
			}
			s.changed = true
		}
	}
	return s.AssetStore.GetAsset(id)
}

func (s *trueNASBindingDriftStore) FindTrueNASActionAssets(collector, kind, vmID string) ([]assets.Asset, error) {
	return s.AssetStore.(persistence.TrueNASActionAssetStore).FindTrueNASActionAssets(collector, kind, vmID)
}

func TestTrueNASActionProductionRuntimeBinding(t *testing.T) {
	var callsA, callsB atomic.Int32
	var lastVM, lastVMB atomic.Int32
	serverA := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		callsA.Add(1)
		if method == "vm.stop" {
			if len(params) == 0 {
				t.Error("missing numeric VM parameter")
			} else if id, ok := params[0].(float64); ok {
				lastVM.Store(int32(id))
			} else {
				t.Errorf("VM parameter is not numeric: %T", params[0])
			}
		} else if method != "system.reboot" {
			t.Errorf("unexpected RPC method %s", method)
		}
		return true, nil
	})
	defer serverA.Close()
	serverB := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		callsB.Add(1)
		if method != "vm.stop" || len(params) == 0 {
			t.Errorf("unexpected B action %s", method)
		} else if id, ok := params[0].(float64); ok {
			lastVMB.Store(int32(id))
		} else {
			t.Errorf("B VM parameter is not numeric: %T", params[0])
		}
		return true, nil
	})
	defer serverB.Close()

	sut := newTestAPIServer(t)
	// Match startup's catalog and status decorator, not a configured test-only registry.
	sut.connectorRegistry = buildConnectorRegistry()
	sut.assetStore = assetstatus.NewStore(sut.assetStore, nil)
	windows := &trueNASActionWindows{}
	sut.groupMaintenanceStore = windows
	createTrueNASCredentialProfile(t, sut, "cred-a", "synthetic-action-key-a", serverA.URL)
	createTrueNASCredentialProfile(t, sut, "cred-b", "synthetic-action-key-b", serverB.URL)
	collector := func(id, credential, baseURL string, enabled bool) hubcollector.Collector {
		return hubcollector.Collector{ID: id, CollectorType: "truenas", Enabled: enabled,
			Config: map[string]any{"base_url": baseURL, "credential_id": credential}}
	}
	inventory := &stubHubCollectorStore{collectors: []hubcollector.Collector{
		collector("nas-a", "cred-a", serverA.URL, true), collector("nas-b", "cred-b", serverB.URL, false),
	}}
	sut.hubCollectorStore = inventory
	named := assetid.ScopeCollectorAssetID("truenas-vm-builder", "nas-a")
	for _, asset := range []assets.HeartbeatRequest{
		{AssetID: named, Source: "truenas", Type: "vm", GroupID: "vm-a", Metadata: map[string]string{"collector_id": "nas-a", "vm_id": "42"}},
		{AssetID: assetid.ScopeCollectorAssetID("truenas-vm-effective", "nas-a"), Source: "truenas", Type: "vm", GroupID: "vm-b", Metadata: map[string]string{"collector_id": "nas-a", "vm_id": "43"}},
		{AssetID: assetid.ScopeCollectorAssetID("truenas-host-test", "nas-a"), Source: "truenas", Type: "nas", GroupID: "nas-host", Metadata: map[string]string{"collector_id": "nas-a"}},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(asset); err != nil {
			t.Fatal(err)
		}
	}
	direct := func(action string, req connectorsdk.ActionRequest, want int) {
		t.Helper()
		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, httptest.NewRequest(http.MethodPost,
			"/connectors/truenas/actions/"+action+"/execute", strings.NewReader(string(body))))
		if rec.Code != want {
			t.Fatalf("direct status=%d body=%s", rec.Code, rec.Body.String())
		}
	}

	for _, tc := range []struct {
		action string
		req    connectorsdk.ActionRequest
		vm     int32
	}{
		{"workload.stop", connectorsdk.ActionRequest{TargetID: named}, 42},
		{"vm.stop", connectorsdk.ActionRequest{TargetID: named, Params: map[string]string{"vm_id": "43"}}, 43},
		{"vm.stop", connectorsdk.ActionRequest{TargetID: "043"}, 43},
	} {
		before := callsA.Load()
		direct(tc.action, tc.req, http.StatusOK)
		if callsA.Load() != before+1 || lastVM.Load() != tc.vm || callsB.Load() != 0 {
			t.Fatalf("wrong provider effect: A=%d B=%d VM=%d", callsA.Load(), callsB.Load(), lastVM.Load())
		}
	}
	windows.blocked = "vm-b"
	before := callsA.Load()
	direct("vm.stop", connectorsdk.ActionRequest{TargetID: named, Params: map[string]string{"vm_id": "43"}}, http.StatusLocked)
	direct("vm.stop", connectorsdk.ActionRequest{TargetID: "43"}, http.StatusLocked)
	windows.blocked = "nas-host"
	direct("system.reboot", connectorsdk.ActionRequest{}, http.StatusLocked)
	if callsA.Load() != before || callsB.Load() != 0 {
		t.Fatal("maintenance denial still called a provider")
	}
	windows.blocked = ""
	direct("system.reboot", connectorsdk.ActionRequest{}, http.StatusOK)
	if callsA.Load() != before+1 || callsB.Load() != 0 {
		t.Fatal("unlocked global action did not reach the selected NAS")
	}

	queue := &trueNASActionQueue{}
	actionDeps := sut.buildActionsDeps()
	actionDeps.JobQueue = queue
	rec := httptest.NewRecorder()
	actionDeps.HandleActionExecute(rec, httptest.NewRequest(http.MethodPost, "/actions/execute",
		strings.NewReader(`{"type":"connector_action","connector_id":"truenas","action_id":"vm.stop","target":"43"}`)))
	if rec.Code != http.StatusAccepted || len(queue.jobs) != 1 || queue.jobs[0].Params["collector_id"] != "nas-a" {
		t.Fatalf("queued owner not pinned: status=%d jobs=%+v body=%s", rec.Code, queue.jobs, rec.Body.String())
	}
	job := queue.jobs[0]
	if job.Target != "43" || job.Params["vm_id"] != "43" {
		t.Fatalf("queued raw target did not retain the actual VM: %+v", job)
	}
	windows.blocked = "vm-b"
	before = callsA.Load()
	result := sut.ensureProxmoxDeps().ExecuteActionInProcess(job)
	if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "maintenance") || callsA.Load() != before {
		t.Fatalf("queued action ignored a new window: %+v", result)
	}
	windows.blocked = ""
	result = sut.ensureProxmoxDeps().ExecuteActionInProcess(job)
	if result.Status != actions.StatusSucceeded || callsA.Load() != before+1 || lastVM.Load() != 43 {
		t.Fatalf("queued configured runtime failed: %+v", result)
	}

	before = callsA.Load()
	unbound := job
	unbound.Params = nil
	result = sut.ensureProxmoxDeps().ExecuteActionInProcess(unbound)
	if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "resubmit") || callsA.Load() != before {
		t.Fatalf("old unbound job guessed its NAS: %+v", result)
	}
	// Both providers are now enabled. Canonical ownership must beat a global
	// singleton count, and a raw action must name its provider explicitly.
	inventory.collectors[1].Enabled = true
	namedB := assetid.ScopeCollectorAssetID("truenas-vm-builder", "nas-b")
	seed := func(id, owner, vm string) {
		t.Helper()
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id, Source: "truenas", Type: "vm", GroupID: "vm-a",
			Metadata: map[string]string{"collector_id": owner, "vm_id": vm},
		}); err != nil {
			t.Fatal(err)
		}
	}
	seed(namedB, "nas-b", "42")
	before, beforeB := callsA.Load(), callsB.Load()
	direct("workload.stop", connectorsdk.ActionRequest{TargetID: named}, http.StatusOK)
	direct("workload.stop", connectorsdk.ActionRequest{TargetID: namedB}, http.StatusOK)
	direct("vm.stop", connectorsdk.ActionRequest{TargetID: "042", Params: map[string]string{"collector_id": "nas-b"}}, http.StatusOK)
	if callsA.Load() != before+1 || callsB.Load() != beforeB+2 || lastVM.Load() != 42 || lastVMB.Load() != 42 {
		t.Fatalf("two-provider effects A=%d B=%d VMs=%d/%d", callsA.Load(), callsB.Load(), lastVM.Load(), lastVMB.Load())
	}
	before, beforeB = callsA.Load(), callsB.Load()
	direct("vm.stop", connectorsdk.ActionRequest{TargetID: "42"}, http.StatusConflict)
	direct("vm.stop", connectorsdk.ActionRequest{TargetID: named, Params: map[string]string{"collector_id": "nas-b"}}, http.StatusConflict)
	direct("vm.stop", connectorsdk.ActionRequest{TargetID: "truenas-vm-missing", Params: map[string]string{"collector_id": "nas-a", "vm_id": "42"}}, http.StatusConflict)
	if callsA.Load() != before || callsB.Load() != beforeB {
		t.Fatal("ambiguous or stale target reached a provider")
	}
	queueAction := func(target string, params map[string]string) actions.Job {
		t.Helper()
		body, err := json.Marshal(actions.ExecuteRequest{Type: actions.RunTypeConnectorAction,
			ConnectorID: "truenas", ActionID: "workload.stop", Target: target, Params: params})
		if err != nil {
			t.Fatal(err)
		}
		count := len(queue.jobs)
		recorder := httptest.NewRecorder()
		actionDeps.HandleActionExecute(recorder, httptest.NewRequest(http.MethodPost, "/actions/execute", strings.NewReader(string(body))))
		if recorder.Code != http.StatusAccepted || len(queue.jobs) != count+1 {
			t.Fatalf("canonical queue status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		return queue.jobs[count]
	}
	canonical := queueAction(named, map[string]string{"_truenas_target_vm_id": "forged"})
	if canonical.Target != named || canonical.ActionID != "workload.stop" || canonical.Params["collector_id"] != "nas-a" ||
		canonical.Params["vm_id"] != "42" || canonical.Params["_truenas_target_vm_id"] != "42" {
		t.Fatalf("canonical queue binding changed audit or provider identity: %+v", canonical)
	}
	override := queueAction(named, map[string]string{"vm_id": "043"})
	second := queueAction(namedB, nil)
	result = sut.ensureProxmoxDeps().ExecuteActionInProcessContext(context.Background(), override)
	if result.Status != actions.StatusSucceeded || callsA.Load() != before+1 || lastVM.Load() != 43 {
		t.Fatalf("queued explicit VM override changed: %+v", result)
	}
	result = sut.ensureProxmoxDeps().ExecuteActionInProcessContext(context.Background(), second)
	if result.Status != actions.StatusSucceeded || callsB.Load() != beforeB+1 || lastVMB.Load() != 42 {
		t.Fatalf("queued second provider failed: %+v", result)
	}
	before, beforeB = callsA.Load(), callsB.Load()
	reject := func(saved actions.Job) {
		t.Helper()
		result := sut.ensureProxmoxDeps().ExecuteActionInProcessContext(context.Background(), saved)
		if result.Status != actions.StatusFailed || callsA.Load() != before || callsB.Load() != beforeB {
			t.Fatalf("stale binding dispatched: %+v A=%d B=%d", result, callsA.Load(), callsB.Load())
		}
	}
	seed(named, "nas-a", "44")
	reject(canonical) // The worker must not resolve the new metadata VM.
	seed(named, "nas-a", "42")
	if err := sut.assetStore.DeleteAsset(named); err != nil {
		t.Fatal(err)
	}
	reject(canonical) // A saved VM ID does not authorize a deleted canonical target.
	seed(named, "nas-a", "42")
	seed("43", "nas-a", "44")
	reject(job) // A formerly raw ID cannot silently become a different asset.
	if err := sut.assetStore.DeleteAsset("43"); err != nil {
		t.Fatal(err)
	}
	result = sut.ensureProxmoxDeps().ExecuteActionInProcessContext(context.Background(), canonical)
	if result.Status != actions.StatusSucceeded || callsA.Load() != before+1 || lastVM.Load() != 42 {
		t.Fatalf("restored canonical binding failed: %+v", result)
	}
	// The raw VM ID exists independently on either NAS. A queued action for A
	// must never follow a replacement singleton B or a cached disabled A runtime.
	inventory.collectors[0].Enabled = false
	inventory.collectors[1].Enabled = true
	before = callsA.Load()
	result = sut.ensureProxmoxDeps().ExecuteActionInProcess(job)
	if result.Status != actions.StatusFailed || callsA.Load() != before || callsB.Load() != beforeB {
		t.Fatalf("queued action escaped its NAS owner: result=%+v A=%d B=%d", result, callsA.Load(), callsB.Load())
	}
	direct("vm.stop", connectorsdk.ActionRequest{TargetID: named}, http.StatusConflict)
	if callsA.Load() != before || callsB.Load() != beforeB {
		t.Fatal("disabled canonical owner fell back to another NAS")
	}
	// Change real stored metadata between authorization's read and binding's
	// fresh read. A restricted request must not queue the stale synthesized VM.
	inventory.collectors[0].Enabled = true
	baseStore := sut.assetStore
	drift := &trueNASBindingDriftStore{AssetStore: baseStore, target: named, change: func() error {
		_, err := baseStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: named, Source: "truenas", Type: "vm", GroupID: "vm-a",
			Metadata: map[string]string{"collector_id": "nas-a", "vm_id": "44"},
		})
		return err
	}}
	sut.assetStore = drift
	actionDeps = sut.buildActionsDeps()
	actionDeps.JobQueue = queue
	queuedBefore := len(queue.jobs)
	body, err := json.Marshal(actions.ExecuteRequest{Type: actions.RunTypeConnectorAction,
		ConnectorID: "truenas", ActionID: "vm.stop", Target: named})
	if err != nil {
		t.Fatal(err)
	}
	restricted := httptest.NewRequest(http.MethodPost, "/actions/execute", strings.NewReader(string(body)))
	restricted = restricted.WithContext(apiv2.ContextWithAllowedAssets(restricted.Context(), []string{named}))
	rec = httptest.NewRecorder()
	actionDeps.HandleActionExecute(rec, restricted)
	if rec.Code != http.StatusConflict || !drift.changed || len(queue.jobs) != queuedBefore || callsA.Load() != before || callsB.Load() != beforeB {
		t.Fatalf("authorization drift queued/dispatched: status=%d changed=%v jobs=%d body=%s", rec.Code, drift.changed, len(queue.jobs), rec.Body.String())
	}
	// An unrestricted request with no explicit VM now binds the current row,
	// preserving one consistent provider ID and metadata baseline in the job.
	rec = httptest.NewRecorder()
	actionDeps.HandleActionExecute(rec, httptest.NewRequest(http.MethodPost, "/actions/execute", strings.NewReader(string(body))))
	if rec.Code != http.StatusAccepted || len(queue.jobs) != queuedBefore+1 {
		t.Fatalf("fresh unrestricted binding failed: %d %s", rec.Code, rec.Body.String())
	}
	fresh := queue.jobs[queuedBefore]
	if fresh.Params["vm_id"] != "44" || fresh.Params["_truenas_target_vm_id"] != "44" || fresh.Target != named {
		t.Fatalf("fresh binding uses mixed metadata: %+v", fresh)
	}
	result = sut.ensureProxmoxDeps().ExecuteActionInProcessContext(context.Background(), fresh)
	if result.Status != actions.StatusSucceeded || callsA.Load() != before+1 || callsB.Load() != beforeB || lastVM.Load() != 44 {
		t.Fatalf("fresh bound VM execution failed: %+v", result)
	}
}
