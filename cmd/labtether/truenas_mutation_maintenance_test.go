package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/assetstatus"
	"github.com/labtether/labtether/internal/auth"
	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
)

type trueNASMutationCall struct {
	method string
	params []any
}
type trueNASMutationFixture struct {
	s        *apiServer
	host     string
	windows  *trueNASActionWindows
	mu       sync.Mutex
	calls    []trueNASMutationCall
	response func(string, []any) (any, *trueNASRPCError, bool)
}

func newTrueNASMutationFixture(t *testing.T) *trueNASMutationFixture {
	t.Helper()
	f := &trueNASMutationFixture{s: newTestAPIServer(t), windows: &trueNASActionWindows{}}
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		f.mu.Lock()
		f.calls = append(f.calls, trueNASMutationCall{method, params})
		response := f.response
		f.mu.Unlock()
		if response != nil {
			if result, err, handled := response(method, params); handled {
				return result, err
			}
		}
		switch method {
		case "pool.scrub.scrub", "replication.run":
			return 777, nil
		case "replication.query":
			return []map[string]any{{"id": 9, "direction": "PUSH", "transport": "SSH", "source_datasets": []string{"tank/data"}, "target_dataset": "remote/data", "recursive": false, "exclude": []string{}}}, nil
		case "system.info":
			return map[string]any{"hostname": "audit-nas"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "tank"}, {"id": 2, "name": "dest"}}, nil
		case "pool.dataset.query":
			return []map[string]any{{"name": "tank/data"}, {"name": "dest/parent"}}, nil
		case "sharing.smb.query":
			return []map[string]any{{"id": 7, "name": "team", "path": "/mnt/tank/data"}}, nil
		case "sharing.nfs.query":
			return []map[string]any{{"id": 8, "path": "/mnt/tank/data"}}, nil
		case "service.query":
			return []map[string]any{{"id": 1, "service": "nfs"}}, nil
		case "vm.query":
			return []map[string]any{{"id": 42, "name": "builder"}}, nil
		case "disk.query":
			return []map[string]any{{"name": "sda"}}, nil
		case "app.query":
			return []map[string]any{}, nil
		default:
			return map[string]any{}, nil
		}
	})
	t.Cleanup(server.Close)
	f.s.connectorRegistry = buildConnectorRegistry()
	f.s.assetStore = assetstatus.NewStore(f.s.assetStore, nil)
	f.s.groupMaintenanceStore = f.windows
	createTrueNASCredentialProfile(t, f.s, "tn-cred", "synthetic-guard-key", server.URL)
	f.s.hubCollectorStore = &stubHubCollectorStore{collectors: []hubcollector.Collector{{
		ID: "nas-a", CollectorType: "truenas", Enabled: true, AssetID: "nas-parent",
		Config: map[string]any{"base_url": server.URL, "credential_id": "tn-cred"},
	}}}
	discovered, err := tnconnector.NewWithConfig(tnconnector.Config{BaseURL: server.URL, APIKey: "synthetic-guard-key"}).Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range discovered {
		group := a.Type
		if a.Type == "dataset" {
			group = a.Name
		}
		if a.Type == "storage-pool" {
			group = a.Name
		}
		a.Metadata["collector_id"] = "nas-a"
		id := assetid.ScopeCollectorAssetID(a.ID, "nas-a")
		if a.Type == "nas" {
			f.host = id
		}
		_, err := f.s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Name: a.Name, Source: a.Source, Type: a.Type, GroupID: group, Metadata: a.Metadata})
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.host == "" {
		t.Fatal("discovery did not produce a NAS owner")
	}
	if _, err := f.s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "nas-parent", Source: "agent", Type: "host", GroupID: "parent"}); err != nil {
		t.Fatal(err)
	}
	f.takeCalls()
	return f
}

func (f *trueNASMutationFixture) takeCalls() []trueNASMutationCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.calls
	f.calls = nil
	return out
}

func (f *trueNASMutationFixture) request(method, path, body string, v2 bool) *httptest.ResponseRecorder {
	prefix := "/truenas/assets/"
	if v2 {
		prefix = "/api/v2/truenas/assets/"
	}
	req := httptest.NewRequest(method, prefix+f.host+"/"+path, strings.NewReader(body))
	req = req.WithContext(contextWithScopes(contextWithPrincipal(req.Context(), "test-admin", auth.RoleAdmin), []string{"connectors:read", "connectors:write"}))
	rec := httptest.NewRecorder()
	if v2 {
		f.s.handleV2TrueNASAssets(rec, req)
	} else {
		f.s.handleTrueNASAssets(rec, req)
	}
	return rec
}

func TestTrueNASDedicatedMutationsHonorApplianceMaintenance(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	for _, tc := range []struct{ method, path, body, rpc string }{
		{"POST", "pools/tank/scrub", "", "pool.scrub.scrub"},
		{"POST", "datasets", `{"name":"tank/new"}`, "pool.dataset.create"},
		{"PUT", "datasets/tank%2Fdata", `{}`, "pool.dataset.update"},
		{"DELETE", "datasets/tank%2Fdata", "", "pool.dataset.delete"},
		{"POST", "snapshots", `{"dataset":"tank/data","name":"check"}`, "zfs.snapshot.create"},
		{"DELETE", "snapshots/tank%2Fdata%40check", "", "zfs.snapshot.delete"},
		{"POST", "snapshots/tank%2Fdata%40check/rollback", "", "zfs.snapshot.rollback"},
		{"POST", "snapshots/tank%2Fdata%40check/clone", `{"dataset_dst":"dest/parent/new"}`, "zfs.snapshot.clone"},
		{"POST", "shares/smb", `{"name":"new","path":"/mnt/tank/data"}`, "sharing.smb.create"},
		{"PUT", "shares/nfs/8", `{"enabled":false}`, "sharing.nfs.update"},
		{"DELETE", "shares/smb/7", "", "sharing.smb.delete"},
		{"POST", "replication/9/run", "", "replication.run"},
		{"POST", "disks/sda/smart-test", "", "smart.test.manual_test"},
		{"POST", "services/nfs/restart", "", "service.restart"},
		{"PUT", "services/nfs/enable", `{"enable":false}`, "service.update"},
		{"POST", "vms/42/restart", "", "vm.restart"},
	} {
		t.Run(tc.rpc, func(t *testing.T) {
			f.windows.blocked = "nas"
			rec := f.request(tc.method, tc.path, tc.body, false)
			calls := f.takeCalls()
			// Service enable performs its read-only ID lookup before the final guard.
			for _, call := range calls {
				if call.method != "service.query" {
					t.Fatalf("blocked RPC dispatched: %+v", call)
				}
			}
			if rec.Code != http.StatusLocked {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			f.windows.blocked = ""
			rec = f.request(tc.method, tc.path, tc.body, false)
			calls = f.takeCalls()
			if rec.Code != http.StatusOK || len(calls) == 0 || calls[len(calls)-1].method != tc.rpc {
				t.Fatalf("open status=%d calls=%+v body=%s", rec.Code, calls, rec.Body.String())
			}
		})
	}
	f.windows.blocked = "parent"
	rec := f.request("POST", "replication/9/run", "", false)
	if calls := f.takeCalls(); rec.Code != http.StatusLocked || len(calls) != 0 {
		t.Fatalf("parent status=%d calls=%+v", rec.Code, calls)
	}
}

func TestTrueNASSecondaryMaintenanceUsesDiscoveredOwners(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	for _, tc := range []struct{ group, method, path, body string }{
		{"tank", "POST", "pools/tank/scrub", ""},
		{"tank/data", "DELETE", "datasets/tank%2Fdata", ""},
		{"tank/data", "POST", "snapshots", `{"dataset":"tank/data","name":"check"}`},
		{"tank/data", "POST", "snapshots/tank%2Fdata%40check/rollback", ""},
		{"dest/parent", "POST", "snapshots/tank%2Fdata%40check/clone", `{"dataset_dst":"dest/parent/new"}`},
		{"dest", "POST", "datasets", `{"name":"dest/new"}`},
		{"service", "POST", "services/nfs/stop", ""},
		{"disk", "POST", "disks/sda/smart-test", ""},
		{"vm", "POST", "vms/42/restart", ""},
	} {
		t.Run(tc.group+tc.path, func(t *testing.T) {
			f.windows.blocked = tc.group
			rec := f.request(tc.method, tc.path, tc.body, false)
			if calls := f.takeCalls(); rec.Code != http.StatusLocked || len(calls) != 0 {
				t.Fatalf("status=%d calls=%+v body=%s", rec.Code, calls, rec.Body.String())
			}
		})
	}
	// A legacy row from another collector is not an owner of this appliance.
	_, err := f.s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "truenas-storage-pool-tank", Source: "truenas", Type: "storage-pool", GroupID: "foreign", Metadata: map[string]string{"collector_id": "nas-b"}})
	if err != nil {
		t.Fatal(err)
	}
	f.windows.blocked = "foreign"
	if rec := f.request("POST", "pools/tank/scrub", "", false); rec.Code != http.StatusOK {
		t.Fatalf("foreign group denied own pool: %s", rec.Body.String())
	}
	if calls := f.takeCalls(); len(calls) != 1 || calls[0].method != "pool.scrub.scrub" {
		t.Fatalf("wrong dispatch: %+v", calls)
	}
}

func TestTrueNASConnectorStorageRechecksQueuedMaintenance(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	direct := func(action string, params map[string]string, want int) {
		t.Helper()
		body, err := json.Marshal(connectorsdk.ActionRequest{TargetID: f.host, Params: params})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		f.s.ensureCollectorsDeps().HandleConnectorActions(rec, httptest.NewRequest(http.MethodPost, "/connectors/truenas/actions/"+action+"/execute", strings.NewReader(string(body))))
		if rec.Code != want {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	f.windows.blocked = "tank"
	direct("pool.scrub", map[string]string{"pool_name": "tank"}, http.StatusLocked)
	f.windows.blocked = "tank/data"
	direct("snapshot.rollback", map[string]string{"snapshot_id": "tank/data@check"}, http.StatusLocked)
	if calls := f.takeCalls(); len(calls) != 0 {
		t.Fatalf("blocked connector dispatched: %+v", calls)
	}
	f.windows.blocked = ""
	queue := &trueNASActionQueue{}
	deps := f.s.buildActionsDeps()
	deps.JobQueue = queue
	rec := httptest.NewRecorder()
	deps.HandleActionExecute(rec, httptest.NewRequest(http.MethodPost, "/actions/execute", strings.NewReader(`{"type":"connector_action","connector_id":"truenas","action_id":"snapshot.rollback","params":{"snapshot_id":"tank/data@check"}}`)))
	if rec.Code != http.StatusAccepted || len(queue.jobs) != 1 {
		t.Fatalf("enqueue=%d jobs=%+v body=%s", rec.Code, queue.jobs, rec.Body.String())
	}
	f.windows.blocked = "tank/data"
	result := f.s.ensureProxmoxDeps().ExecuteActionInProcess(queue.jobs[0])
	if result.Status != "failed" || !strings.Contains(result.Error, "maintenance") {
		t.Fatalf("queued lock not enforced: %+v", result)
	}
	if calls := f.takeCalls(); len(calls) != 0 {
		t.Fatalf("queued lock dispatched: %+v", calls)
	}
	f.windows.blocked = ""
	result = f.s.ensureProxmoxDeps().ExecuteActionInProcess(queue.jobs[0])
	calls := f.takeCalls()
	if result.Status != "succeeded" || len(calls) != 1 || calls[0].method != "zfs.snapshot.rollback" || !reflect.DeepEqual(calls[0].params, []any{"tank/data@check"}) {
		t.Fatalf("open queued result=%+v calls=%+v", result, calls)
	}
}

func TestTrueNASStorageRoutesPreserveEncodedProviderIDs(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	for _, v2 := range []bool{false, true} {
		for _, tc := range []struct{ method, path, rpc, id string }{
			{"DELETE", "datasets/" + url.PathEscape("tank/data/child"), "pool.dataset.delete", "tank/data/child"},
			{"DELETE", "snapshots/" + url.PathEscape("tank/data@check"), "zfs.snapshot.delete", "tank/data@check"},
			{"POST", "snapshots/" + url.PathEscape("tank/data@check") + "/rollback", "zfs.snapshot.rollback", "tank/data@check"},
		} {
			rec := f.request(tc.method, tc.path, "", v2)
			calls := f.takeCalls()
			if rec.Code != http.StatusOK || len(calls) != 1 || calls[0].method != tc.rpc || !reflect.DeepEqual(calls[0].params, []any{tc.id}) {
				t.Fatalf("v2=%v status=%d calls=%+v body=%s", v2, rec.Code, calls, rec.Body.String())
			}
		}
	}
	rec := f.request("DELETE", "datasets/tank/data", "", false)
	if calls := f.takeCalls(); rec.Code != http.StatusBadRequest || len(calls) != 0 {
		t.Fatalf("unescaped path silently retargeted: status=%d calls=%+v", rec.Code, calls)
	}
}

func TestTrueNASShareSubtypeRoutesReturnRequestedList(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	f.windows.blocked = "nas" // Maintenance pauses writes, not reads.
	for _, kind := range []string{"smb", "nfs"} {
		rec := f.request("GET", "shares/"+kind, "", false)
		calls := f.takeCalls()
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		var shares []map[string]any
		if err := json.Unmarshal(payload[kind], &shares); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || len(shares) != 1 || len(calls) != 1 || calls[0].method != "sharing."+kind+".query" {
			t.Fatalf("status=%d calls=%+v body=%s", rec.Code, calls, rec.Body.String())
		}
	}
}
