package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/assetstatus"
	"github.com/labtether/labtether/internal/hubcollector"
)

type pbsMaintenanceFixture struct {
	s                                                  *apiServer
	host                                               string
	windows                                            *trueNASActionWindows
	mu                                                 sync.Mutex
	writes                                             []string
	reads                                              []string
	jobStore, remote, remoteStore, taskType, taskStore string
}

func newPBSMaintenanceFixture(t *testing.T) *pbsMaintenanceFixture {
	t.Helper()
	f := &pbsMaintenanceFixture{s: newTestAPIServer(t), windows: &trueNASActionWindows{}, jobStore: "Backup.Main", remote: "offsite", remoteStore: "remote-data", taskType: "verify", taskStore: "Backup.Main"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method != http.MethodGet {
			_ = r.ParseForm()
			f.writes = append(f.writes, r.Method+" "+r.URL.Path+"?"+r.PostForm.Encode())
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:accepted"})
			return
		}
		f.reads = append(f.reads, r.URL.Path)
		var data any
		switch r.URL.Path {
		case "/api2/json/config/verify", "/api2/json/config/prune", "/api2/json/config/sync":
			data = []map[string]any{{"id": "job", "store": f.jobStore, "remote": f.remote, "remote-store": f.remoteStore}}
		case "/api2/json/nodes/localhost/tasks/fixture/status":
			data = map[string]any{"upid": "fixture", "type": f.taskType, "id": f.taskStore, "status": "running"}
		case "/api2/json/admin/datastore/Backup.Main/status":
			data = map[string]any{"store": "Backup.Main"}
		default:
			t.Errorf("unexpected PBS read %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(server.Close)
	createPBSCredentialProfile(t, f.s, "pbs-maintenance", "fixture@pam!audit", "synthetic-secret", server.URL)
	f.s.hubCollectorStore = &stubHubCollectorStore{collectors: []hubcollector.Collector{{
		ID: "pbs-a", CollectorType: "pbs", Enabled: true, AssetID: "pbs-parent",
		Config: map[string]any{"base_url": server.URL, "credential_id": "pbs-maintenance", "token_id": "fixture@pam!audit"},
	}}}
	f.s.assetStore = assetstatus.NewStore(f.s.assetStore, nil)
	f.s.groupMaintenanceStore = f.windows
	f.host = assetid.ScopeCollectorAssetID("pbs-server-fixture", "pbs-a")
	seed := func(id, kind, group string, metadata map[string]string) {
		if _, err := f.s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: kind, Source: "pbs", GroupID: group, Metadata: metadata}); err != nil {
			t.Fatal(err)
		}
	}
	seed(f.host, "storage-controller", "server", map[string]string{"collector_id": "pbs-a"})
	seed("pbs-parent", "host", "parent", nil)
	for _, store := range []string{"Backup.Main", "destination", "local-source", "unrelated"} {
		seed(assetid.ScopeCollectorAssetID("pbs-datastore-"+strings.ToLower(store), "pbs-a"), "storage-pool", store, map[string]string{"collector_id": "pbs-a", "store": store})
	}
	// A legacy row belongs to a different appliance despite its identical
	// provider store. Its paused group must not block the selected appliance.
	seed("pbs-datastore-legacy", "storage-pool", "foreign", map[string]string{"collector_id": "pbs-b", "store": "Backup.Main"})
	return f
}

func (f *pbsMaintenanceFixture) request(method, path, form string, owner bool) *httptest.ResponseRecorder {
	if !strings.HasPrefix(path, "/pbs/tasks/") {
		path = "/pbs/assets/" + f.host + "/" + path
	}
	r := httptest.NewRequest(method, path, strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if owner {
		r = r.WithContext(contextWithPrincipal(r.Context(), "owner", "owner"))
	}
	w := httptest.NewRecorder()
	if strings.HasPrefix(path, "/pbs/tasks/") {
		f.s.handlePBSTaskRoutes(w, r)
	} else {
		f.s.handlePBSAssets(w, r)
	}
	return w
}

func (f *pbsMaintenanceFixture) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

func TestPBSDedicatedWritesHonorExactMaintenanceOwners(t *testing.T) {
	f := newPBSMaintenanceFixture(t)
	type action struct{ method, path, form, group string }
	cases := []action{
		{"POST", "datastores/Backup.Main/gc", "", "Backup.Main"},
		{"POST", "datastores/Backup.Main/verify", "", "Backup.Main"},
		{"POST", "datastores/Backup.Main/maintenance", "mode=offline", "Backup.Main"},
		{"POST", "datastores/Backup.Main/maintenance-enable", "", "Backup.Main"},
		{"POST", "datastores/Backup.Main/maintenance-disable", "", "Backup.Main"},
		{"POST", "snapshots/verify?store=Backup.Main", "", "Backup.Main"},
		{"DELETE", "snapshots/forget?store=Backup.Main&backup-type=vm&backup-id=900&backup-time=123", "", "Backup.Main"},
		{"DELETE", "groups/forget?store=Backup.Main&backup-type=vm&backup-id=900", "", "Backup.Main"},
		{"POST", "remotes", "name=other", "server"},
		{"POST", "traffic-control", "name=rule", "server"},
		{"PUT", "traffic-control/rule", "rate-in=1", "server"},
		{"DELETE", "traffic-control/rule", "", "server"},
		{"POST", "/pbs/tasks/localhost/fixture/stop?collector_id=pbs-a", "", "Backup.Main"},
	}
	for _, kind := range []string{"verify", "prune", "sync"} {
		for _, a := range []action{{"POST", kind + "-jobs", "store=Backup.Main&remote=offsite", "Backup.Main"}, {"PUT", kind + "-jobs/job", "comment=new", "Backup.Main"}, {"DELETE", kind + "-jobs/job", "", "Backup.Main"}, {"POST", kind + "-jobs/job/run", "", "Backup.Main"}} {
			cases = append(cases, a)
		}
	}
	for _, a := range cases {
		t.Run(a.method+" "+a.path, func(t *testing.T) {
			before := f.writeCount()
			f.windows.blocked = a.group
			w := f.request(a.method, a.path, a.form, true)
			if w.Code != http.StatusLocked || f.writeCount() != before {
				t.Fatalf("paused owner: status=%d writes=%d body=%s", w.Code, f.writeCount()-before, w.Body.String())
			}
			f.windows.blocked = "unrelated"
			w = f.request(a.method, a.path, a.form, true)
			if w.Code != http.StatusOK || f.writeCount() != before+1 {
				t.Fatalf("unrelated owner: status=%d writes=%d body=%s", w.Code, f.writeCount()-before, w.Body.String())
			}
		})
	}
	for _, group := range []string{"server", "parent", "foreign"} {
		f.windows.blocked = group
		before := f.writeCount()
		w := f.request("POST", "datastores/Backup.Main/verify", "", true)
		if group == "foreign" {
			if w.Code != http.StatusOK || f.writeCount() != before+1 {
				t.Fatalf("foreign owner blocked selected PBS: %d %s", w.Code, w.Body.String())
			}
		} else if w.Code != http.StatusLocked || f.writeCount() != before {
			t.Fatalf("server/parent guard failed: %s %d", group, w.Code)
		}
	}
	f.windows.blocked = "Backup.Main"
	for _, path := range []string{"datastores/Backup.Main/gc", "verify-jobs", "prune-jobs", "sync-jobs"} {
		if w := f.request("GET", path, "", false); w.Code != http.StatusOK {
			t.Fatalf("paused read blocked: %s %d", path, w.Code)
		}
	}
	// A dry-run simulation stays available and carries the provider dry-run bit.
	w := f.request("POST", "prune-jobs/job/simulate", "", true)
	if w.Code != http.StatusOK {
		t.Fatalf("dry run denied: %d %s", w.Code, w.Body.String())
	}
	f.mu.Lock()
	last := f.writes[len(f.writes)-1]
	f.mu.Unlock()
	if !strings.Contains(last, "dry-run=1") {
		t.Fatalf("simulation lost dry-run: %s", last)
	}
	f.windows.blocked = ""
	before := f.writeCount()
	if w = f.request("POST", "datastores/Backup.Main/verify", "", false); w.Code == http.StatusOK || f.writeCount() != before {
		t.Fatal("maintenance wiring bypassed admin check")
	}
}

func TestPBSJobEditsGuardCurrentAndNewLocalStores(t *testing.T) {
	f := newPBSMaintenanceFixture(t)
	for _, kind := range []string{"verify", "prune", "sync"} {
		for _, group := range []string{"Backup.Main", "destination"} {
			f.windows.blocked = group
			w := f.request("PUT", kind+"-jobs/job", "store=destination", true)
			if w.Code != http.StatusLocked || f.writeCount() != 0 {
				t.Fatalf("%s change missed %s: %d %s", kind, group, w.Code, w.Body.String())
			}
		}
	}
	f.mu.Lock()
	f.remoteStore = "local-source"
	f.mu.Unlock()
	f.windows.blocked = "local-source"
	w := f.request("PUT", "sync-jobs/job", url.Values{"delete": {"remote"}}.Encode(), true)
	if w.Code != http.StatusLocked || f.writeCount() != 0 {
		t.Fatalf("remote deletion missed new local source: %d %s", w.Code, w.Body.String())
	}
	f.mu.Lock()
	f.remote = ""
	f.mu.Unlock()
	w = f.request("POST", "sync-jobs/job/run", "", true)
	if w.Code != http.StatusLocked || f.writeCount() != 0 {
		t.Fatalf("local sync source missed: %d %s", w.Code, w.Body.String())
	}
	f.mu.Lock()
	f.remote = "offsite"
	f.mu.Unlock()
	w = f.request("POST", "sync-jobs/job/run", "", true)
	if w.Code != http.StatusOK || f.writeCount() != 1 {
		t.Fatalf("remote name treated as local: %d %s", w.Code, w.Body.String())
	}
	f.windows.blocked = ""
	for _, path := range []string{"verify-jobs/missing/run", "prune-jobs/missing/run", "sync-jobs/missing/run"} {
		w = f.request("POST", path, "", true)
		if w.Code != http.StatusConflict || f.writeCount() != 1 {
			t.Fatalf("unknown job dispatched: %s %d", path, w.Code)
		}
	}
	f.mu.Lock()
	f.taskType = "unknown-provider-worker"
	f.mu.Unlock()
	w = f.request("POST", "/pbs/tasks/localhost/fixture/stop?collector_id=pbs-a", "", true)
	if w.Code != http.StatusConflict || f.writeCount() != 1 {
		t.Fatalf("unknown task identity dispatched: %d", w.Code)
	}
}
