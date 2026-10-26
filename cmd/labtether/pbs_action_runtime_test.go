package main

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/assetstatus"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
)

// The startup catalog has no configured PBS client. A real direct request and
// the queued worker must use the same captured runtime, not that catalog or env.
func TestPBSActionProductionRuntimeBinding(t *testing.T) {
	sut := newTestAPIServer(t)
	allowInsecureTransportForConnectorTests(t)
	var mu sync.Mutex
	var writes []string
	serve := func(owner string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("unexpected PBS method %s", r.Method)
			}
			mu.Lock()
			writes = append(writes, owner+" "+r.URL.Path)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:fixture"})
		}))
	}
	a, b := serve("A"), serve("B")
	t.Cleanup(a.Close)
	t.Cleanup(b.Close)
	// Deliberately point the old environment-owned catalog at the wrong server.
	// B also has its own working configured runtime as a positive control.
	t.Setenv("PBS_BASE_URL", b.URL)
	t.Setenv("PBS_TOKEN_ID", "fixture@pam!environment")
	t.Setenv("PBS_TOKEN_SECRET", "synthetic-environment-secret")
	sut.connectorRegistry = buildConnectorRegistry()
	sut.assetStore = assetstatus.NewStore(sut.assetStore, nil)
	windows := &trueNASActionWindows{}
	sut.groupMaintenanceStore = windows
	createPBSCredentialProfile(t, sut, "pbs-action-a", "fixture@pam!a", "synthetic-a", a.URL)
	createPBSCredentialProfile(t, sut, "pbs-action-b", "fixture@pam!b", "synthetic-b", b.URL)
	inventory := &stubHubCollectorStore{collectors: []hubcollector.Collector{
		{ID: "pbs-a", CollectorType: "pbs", Enabled: true, Config: map[string]any{"base_url": a.URL, "credential_id": "pbs-action-a"}},
		{ID: "pbs-b", CollectorType: "pbs", Enabled: true, Config: map[string]any{"base_url": b.URL, "credential_id": "pbs-action-b"}},
	}}
	sut.hubCollectorStore = inventory
	seed := func(id, kind, owner, store, group string) {
		t.Helper()
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id, Type: kind, Source: "pbs", Name: store, GroupID: group,
			Metadata: map[string]string{"collector_id": owner, "store": store},
		}); err != nil {
			t.Fatal(err)
		}
	}
	targetA := assetid.ScopeCollectorAssetID("pbs-datastore-backup-main", "pbs-a")
	targetB := assetid.ScopeCollectorAssetID("pbs-datastore-backup-main", "pbs-b")
	seed(targetA, "storage-pool", "pbs-a", "Backup.Main", "store-a")
	seed(targetB, "storage-pool", "pbs-b", "Backup.Main", "store-b")
	seed(assetid.ScopeCollectorAssetID("pbs-datastore-secondary", "pbs-a"), "storage-pool", "pbs-a", "Backup.Secondary", "secondary")
	for _, owner := range []string{"pbs-a", "pbs-b"} {
		seed(assetid.ScopeCollectorAssetID("pbs-server-fixture", owner), "storage-controller", owner, "", "server-"+owner)
	}
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(writes) }
	checkWrite := func(before int, want string) {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		if len(writes) != before+1 || writes[len(writes)-1] != want {
			t.Fatalf("provider effect=%v, want one new %q", writes, want)
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
			"/connectors/pbs/actions/"+action+"/execute", strings.NewReader(string(body))))
		if rec.Code != want {
			t.Fatalf("direct status=%d want=%d body=%s", rec.Code, want, rec.Body.String())
		}
	}
	for _, tc := range []struct {
		action, target, collector, store, want string
	}{
		{"storage.datastore.verify", targetA, "", "", "A /api2/json/admin/datastore/Backup.Main/verify"},
		{"datastore.gc", "Backup.Main", "pbs-a", "", "A /api2/json/admin/datastore/Backup.Main/gc"},
		{"datastore.verify", targetA, "", "Backup.Secondary", "A /api2/json/admin/datastore/Backup.Secondary/verify"},
		{"datastore.prune", targetB, "", "", "B /api2/json/admin/datastore/Backup.Main/prune-datastore"},
		{"datastore.gc", "Backup.Main", "pbs-b", "", "B /api2/json/admin/datastore/Backup.Main/gc"},
	} {
		before := count()
		direct(tc.action, connectorsdk.ActionRequest{TargetID: tc.target,
			Params: map[string]string{"collector_id": tc.collector, "store": tc.store}}, http.StatusOK)
		checkWrite(before, tc.want)
	}
	before := count()
	for _, req := range []connectorsdk.ActionRequest{
		{TargetID: targetA, Params: map[string]string{"collector_id": "pbs-b"}},
		{TargetID: "Backup.Main"}, // Two configured providers; no arbitrary singleton.
		{TargetID: "pbs-datastore-missing", Params: map[string]string{"collector_id": "pbs-a", "store": "Backup.Main"}},
		{TargetID: targetA, Params: map[string]string{"store": "not/a/store"}},
	} {
		direct("datastore.verify", req, http.StatusConflict)
	}
	windows.blocked = "secondary"
	direct("datastore.verify", connectorsdk.ActionRequest{TargetID: targetA, Params: map[string]string{"store": "Backup.Secondary"}}, http.StatusLocked)
	if count() != before {
		t.Fatal("rejected direct request still dispatched")
	}
	windows.blocked = ""

	queue := &trueNASActionQueue{}
	actionDeps := sut.buildActionsDeps()
	actionDeps.JobQueue = queue
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(actions.ExecuteRequest{Type: actions.RunTypeConnectorAction, ConnectorID: "pbs", ActionID: "storage.datastore.verify",
		Target: targetA, Params: map[string]string{"_pbs_target_store": "caller-supplied-forgery"}})
	actionDeps.HandleActionExecute(rec, httptest.NewRequest(http.MethodPost, "/actions/execute", strings.NewReader(string(body))))
	if rec.Code != http.StatusAccepted || len(queue.jobs) != 1 {
		t.Fatalf("queue status=%d jobs=%d body=%s", rec.Code, len(queue.jobs), rec.Body.String())
	}
	job := queue.jobs[0]
	if job.Target != targetA || job.ActionID != "storage.datastore.verify" || job.Params["collector_id"] != "pbs-a" ||
		job.Params["store"] != "Backup.Main" || job.Params["_pbs_target_store"] != "Backup.Main" {
		t.Fatalf("queue lost audit/provider identity: %+v", job)
	}
	execute := func(job actions.Job, wantStatus string) actions.Result {
		t.Helper()
		result := sut.ensureProxmoxDeps().ExecuteActionInProcessContext(context.Background(), job)
		if result.Status != wantStatus {
			t.Fatalf("worker result=%+v, want %s", result, wantStatus)
		}
		return result
	}
	before = count()
	windows.blocked = "store-a"
	if result := execute(job, actions.StatusFailed); !strings.Contains(result.Error, "maintenance") {
		t.Fatalf("worker did not report new maintenance window: %+v", result)
	}
	if count() != before {
		t.Fatal("new maintenance window did not stop queued dispatch")
	}
	windows.blocked = ""
	execute(job, actions.StatusSucceeded)
	checkWrite(before, "A /api2/json/admin/datastore/Backup.Main/verify")

	before = count()
	unbound := job
	unbound.Params = nil
	if result := execute(unbound, actions.StatusFailed); !strings.Contains(result.Error, "resubmit") {
		t.Fatalf("old unbound job did not require resubmission: %+v", result)
	}
	// A queued target cannot silently become another raw store on the same server.
	seed(targetA, "storage-pool", "pbs-a", "New.Store", "store-a")
	execute(job, actions.StatusFailed)
	seed(targetA, "storage-pool", "pbs-a", "Backup.Main", "store-a")
	// A canonical target deleted after enqueue must not turn into a raw alias.
	if err := sut.assetStore.DeleteAsset(targetA); err != nil {
		t.Fatal(err)
	}
	execute(job, actions.StatusFailed)
	seed(targetA, "storage-pool", "pbs-a", "Backup.Main", "store-a")
	// Losing the saved collector cannot select B, even though B is usable and
	// the old catalog's environment is deliberately configured for it.
	inventory.collectors[0].Enabled = false
	execute(job, actions.StatusFailed)
	direct("datastore.verify", connectorsdk.ActionRequest{TargetID: targetA}, http.StatusConflict)
	if count() != before {
		t.Fatal("stale or unbound queued job reached a provider")
	}
	inventory.collectors[0].Enabled = true
	// The request snapshot is independent of the caller's mutable params.
	params := map[string]string{"collector_id": "pbs-a", "store": "Backup.Main"}
	bound, err := sut.ensurePBSDeps().BindAction(context.Background(), "datastore.gc", connectorsdk.ActionRequest{TargetID: "Backup.Main", Params: params})
	if err != nil {
		t.Fatal(err)
	}
	params["collector_id"], params["store"] = "pbs-b", "Backup.Secondary"
	result, err := sut.ensurePBSDeps().ExecuteAction(context.Background(), "datastore.gc", bound)
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("bound request execution=%+v/%v", result, err)
	}
	checkWrite(before, "A /api2/json/admin/datastore/Backup.Main/gc")
	// Current asset ownership still wins over a saved contradictory collector.
	wrong := job
	wrong.Params = maps.Clone(job.Params)
	wrong.Params["collector_id"] = "pbs-b"
	before = count()
	execute(wrong, actions.StatusFailed)
	if count() != before {
		t.Fatal("worker ignored conflicting saved asset owner")
	}
}
