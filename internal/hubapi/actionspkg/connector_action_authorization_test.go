package actionspkg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	proxmoxconnector "github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
)

type actionCaptureQueue struct{ jobs []actions.Job }

func (q *actionCaptureQueue) Enqueue(_ context.Context, kind jobqueue.JobKind, payload []byte) (string, error) {
	var job actions.Job
	if err := json.Unmarshal(payload, &job); err != nil {
		return "", err
	}
	q.jobs = append(q.jobs, job)
	return "queued-action", nil
}

func connectorActionFixture(t *testing.T, asset assets.Asset) (*Deps, *actionCaptureQueue) {
	t.Helper()
	store := persistence.NewMemoryAssetStore()
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: asset.ID, Name: asset.Name, Source: asset.Source, Type: asset.Type,
		Metadata: asset.Metadata, GroupID: asset.GroupID,
	}); err != nil {
		t.Fatal(err)
	}
	queue := &actionCaptureQueue{}
	return &Deps{AssetStore: store, ActionStore: persistence.NewMemoryActionStore(), JobQueue: queue}, queue
}

func executeConnectorRequest(t *testing.T, deps *Deps, request actions.ExecuteRequest, allowed []string) *httptest.ResponseRecorder {
	t.Helper()
	request.Type = actions.RunTypeConnectorAction
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	deps.HandleActionExecute(recorder, savedActionRequest(http.MethodPost, "/actions/execute", string(body), allowed))
	return recorder
}

func TestConnectorActionRestrictedTargetCannotEscapeBeforeQueue(t *testing.T) {
	nas := assets.Asset{ID: "truenas-vm-build-server", Source: "truenas", Type: "vm", Metadata: map[string]string{"vm_id": "42"}}
	vm := assets.Asset{ID: "proxmox-vm-100", Source: "proxmox", Type: "vm", Metadata: map[string]string{"vmid": "100", "node": "node-a", "collector_id": "collector-a"}}
	container := assets.Asset{ID: "docker-ct-agent-a-0123456789ab", Source: "docker", Type: "docker-container"}
	portainer := assets.Asset{ID: "portainer-container-1-0123456789ab", Source: "portainer", Type: "container"}
	for _, tc := range []struct {
		name, connector, action, target string
		asset                           assets.Asset
		params                          map[string]string
		allowed                         []string
	}{
		{"other asset", "truenas", "vm.start", nas.ID, nas, nil, []string{"another-asset"}},
		{"blank target", "truenas", "vm.start", "", nas, map[string]string{"vm_id": "42"}, []string{nas.ID}},
		{"raw provider reference", "truenas", "vm.start", "42", nas, nil, []string{"42"}},
		{"VM override", "truenas", "vm.start", nas.ID, nas, map[string]string{"vm_id": "43"}, []string{nas.ID}},
		{"global action", "truenas", "system.reboot", nas.ID, nas, nil, []string{nas.ID}},
		{"secondary resource", "truenas", "snapshot.delete", nas.ID, nas, map[string]string{"snapshot_id": "pool/other@snapshot"}, []string{nas.ID}},
		{"wrong connector source", "proxmox", "vm.start", nas.ID, nas, nil, []string{nas.ID}},
		{"missing provider metadata", "truenas", "vm.start", "truenas-vm-lost", assets.Asset{ID: "truenas-vm-lost", Source: "truenas", Type: "vm"}, nil, []string{"truenas-vm-lost"}},
		{"collector override", "proxmox", "vm.start", vm.ID, vm, map[string]string{"collector_id": "collector-b"}, []string{vm.ID}},
		{"unknown target param", "proxmox", "vm.start", vm.ID, vm, map[string]string{"vmid": "200"}, []string{vm.ID}},
		{"wrong workload kind", "proxmox", "ct.start", vm.ID, vm, nil, []string{vm.ID}},
		{"migration", "proxmox", "vm.migrate", vm.ID, vm, map[string]string{"target_node": "node-b"}, []string{vm.ID}},
		{"clone", "proxmox", "vm.clone", vm.ID, vm, map[string]string{"new_id": "200"}, []string{vm.ID}},
		{"new container", "docker", "container.create", container.ID, container, map[string]string{"image": "other"}, []string{container.ID}},
		{"container override", "docker", "container.start", container.ID, container, map[string]string{"container_id": "other"}, []string{container.ID}},
		{"endpoint override", "portainer", "container.start", portainer.ID, portainer, map[string]string{"endpoint_id": "2"}, []string{portainer.ID}},
		{"unknown operation", "portainer", "container.future", portainer.ID, portainer, nil, []string{portainer.ID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps, queue := connectorActionFixture(t, tc.asset)
			recorder := executeConnectorRequest(t, deps, actions.ExecuteRequest{ConnectorID: tc.connector, ActionID: tc.action, Target: tc.target, Params: tc.params}, tc.allowed)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status=%d: %s", recorder.Code, recorder.Body.String())
			}
			runs, err := deps.ActionStore.ListActionRuns(100, 0, "", "")
			if err != nil || len(runs) != 0 || len(queue.jobs) != 0 {
				t.Fatalf("rejected action reached persistence/queue: runs=%d jobs=%d err=%v", len(runs), len(queue.jobs), err)
			}
		})
	}
}

func TestConnectorActionRestrictedSingleAssetPreservesCanonicalTarget(t *testing.T) {
	for _, tc := range []struct {
		asset  assets.Asset
		action string
		params map[string]string
	}{
		{assets.Asset{ID: "proxmox-vm-100", Source: "proxmox", Type: "vm", Metadata: map[string]string{"vmid": "100", "node": "node-a", "collector_id": "collector-a"}}, "vm.disk_resize", map[string]string{"disk": "scsi0", "size": "+10G", "collector_id": "collector-a"}},
		{assets.Asset{ID: "proxmox-ct-101", Source: "proxmox", Type: "container", Metadata: map[string]string{"vmid": "101", "node": "node-a", "proxmox_type": "lxc"}}, "ct.snapshot.rollback", map[string]string{"snapshot_name": "before-update"}},
		{assets.Asset{ID: "docker-ct-agent-a-0123456789ab", Source: "docker", Type: "docker-container"}, "container.restart", map[string]string{"timeout": "10"}},
		{assets.Asset{ID: assetid.ScopeCollectorAssetID("portainer-container-1-0123456789ab", "collector-a"), Source: "portainer", Type: "container"}, "container.remove", map[string]string{"force": "true"}},
	} {
		t.Run(tc.asset.Source+"/"+tc.action, func(t *testing.T) {
			tc.asset.GroupID = "group-a"
			deps, queue := connectorActionFixture(t, tc.asset)
			groups := &groupfeatures.Deps{AssetStore: deps.AssetStore}
			deps.ResolveGroupIDForAction = groups.ResolveGroupIDForAction
			guarded := ""
			deps.EvaluateGuardrails = func(groupID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
				guarded = groupID
				return groupfeatures.GroupMaintenanceGuardrails{}, nil
			}
			var events []audit.Event
			deps.AppendAuditEventBestEffort = func(event audit.Event, _ string) { events = append(events, event) }
			recorder := executeConnectorRequest(t, deps, actions.ExecuteRequest{ConnectorID: tc.asset.Source, ActionID: tc.action, Target: tc.asset.ID, Params: tc.params}, []string{tc.asset.ID})
			if recorder.Code != http.StatusAccepted || len(queue.jobs) != 1 {
				t.Fatalf("status=%d jobs=%d: %s", recorder.Code, len(queue.jobs), recorder.Body.String())
			}
			if queue.jobs[0].Target != tc.asset.ID || guarded != "group-a" || len(events) != 2 {
				t.Fatalf("canonical target/group lost: job=%+v group=%q events=%+v", queue.jobs[0], guarded, events)
			}
			for _, event := range events {
				if event.Target != tc.asset.ID {
					t.Fatalf("audit target=%q", event.Target)
				}
			}
		})
	}
}

func TestConnectorActionTrueNASNamedVMReachesStoredProviderID(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	type rpcCall struct {
		ID     int             `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	calls := make(chan rpcCall, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var call rpcCall
		if err := conn.ReadJSON(&call); err != nil {
			t.Error(err)
			return
		}
		if call.Method != "auth.login_with_api_key" {
			t.Errorf("unexpected auth: %s", call.Method)
			return
		}
		if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": true}); err != nil {
			t.Error(err)
			return
		}
		if err := conn.ReadJSON(&call); err != nil {
			t.Error(err)
			return
		}
		calls <- call
		if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": true}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	registry := connectorsdk.NewRegistry()
	registry.Register(truenas.NewWithConfig(truenas.Config{BaseURL: server.URL, APIKey: "fixture-key", Timeout: 3 * time.Second}))
	asset := assets.Asset{ID: assetid.ScopeCollectorAssetID("truenas-vm-build-server", "collector-a"), Source: "truenas", Type: "vm", Metadata: map[string]string{"vm_id": "42"}}
	for _, tc := range []struct {
		name, action, override string
		restricted             bool
		want                   string
	}{
		{"restricted named start", "vm.start", "", true, "[42]"},
		{"restricted alias stop", "workload.stop", "", true, "[42]"},
		{"unrestricted named start", "vm.start", "", false, "[42]"},
		{"unrestricted explicit override", "vm.start", "43", false, "[43]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps, queue := connectorActionFixture(t, asset)
			deps.ConnectorRegistry = registry
			var allowed []string
			if tc.restricted {
				allowed = []string{asset.ID}
			}
			recorder := executeConnectorRequest(t, deps, actions.ExecuteRequest{ConnectorID: "truenas", ActionID: tc.action, Target: asset.ID, Params: map[string]string{"vm_id": tc.override}}, allowed)
			if recorder.Code != http.StatusAccepted || len(queue.jobs) != 1 {
				t.Fatalf("status=%d: %s", recorder.Code, recorder.Body.String())
			}
			job := queue.jobs[0]
			if job.Target != asset.ID {
				t.Fatalf("canonical target replaced: %q", job.Target)
			}
			result := operations.ExecuteActionInProcess(job, registry)
			if result.Status != actions.StatusSucceeded {
				t.Fatalf("provider action failed: %+v", result)
			}
			select {
			case call := <-calls:
				wantMethod := "vm.start"
				if strings.HasSuffix(tc.action, ".stop") {
					wantMethod = "vm.stop"
				}
				if call.Method != wantMethod || string(call.Params) != tc.want {
					t.Fatalf("provider got %s %s, want %s %s", call.Method, call.Params, wantMethod, tc.want)
				}
			default:
				t.Fatal("action never reached provider")
			}
		})
	}
}

func TestConnectorActionUnrestrictedProviderReferencesRemainSupported(t *testing.T) {
	for _, req := range []actions.ExecuteRequest{
		{ConnectorID: "truenas", ActionID: "system.reboot"},
		{ConnectorID: "truenas", ActionID: "vm.start", Target: "42"},
		{ConnectorID: "proxmox", ActionID: "vm.migrate", Target: "node-a/100", Params: map[string]string{"target_node": "node-b"}},
	} {
		deps := &Deps{ActionStore: persistence.NewMemoryActionStore(), JobQueue: &actionCaptureQueue{}}
		recorder := executeConnectorRequest(t, deps, req, nil)
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("unrestricted %s: %d %s", req.ActionID, recorder.Code, recorder.Body.String())
		}
	}
}

func TestConnectorActionCanonicalAliasUsesAuthorizedProviderOperation(t *testing.T) {
	asset := assets.Asset{ID: "proxmox-ct-101", Source: "proxmox", Type: "container", Metadata: map[string]string{"vmid": "101", "node": "node-a", "proxmox_type": "lxc"}}
	deps, queue := connectorActionFixture(t, asset)
	deps.ConnectorRegistry = connectorsdk.NewRegistry()
	deps.ConnectorRegistry.Register(proxmoxconnector.New())
	recorder := executeConnectorRequest(t, deps, actions.ExecuteRequest{ConnectorID: "proxmox", ActionID: "workload.start", Target: asset.ID}, []string{asset.ID})
	if recorder.Code != http.StatusAccepted || len(queue.jobs) != 1 || queue.jobs[0].ActionID != "ct.start" {
		t.Fatalf("alias escaped authorized operation: status=%d jobs=%+v", recorder.Code, queue.jobs)
	}
}
