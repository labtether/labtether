package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// Actual HTTP routes, queue binding and worker dispatch share the configured
// provider owner. Startup env must not select the other usable HA instance.
func TestHomeAssistantActionProductionRuntimeBinding(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	var mu sync.Mutex
	type effect struct {
		owner, path string
		body        map[string]string
	}
	var effects []effect
	server := func(owner string) *httptest.Server {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/services/") || r.Header.Get("Authorization") != "Bearer synthetic-"+owner {
				t.Errorf("wrong HA provider request %s %s", r.Method, r.URL.Path)
			}
			for key := range body {
				if strings.HasPrefix(key, "_ha_") || key == "collector_id" || key == "group_id" || key == "requested_actor_label" {
					t.Errorf("Hub routing/audit parameter leaked to provider: %s", key)
				}
			}
			mu.Lock()
			effects = append(effects, effect{owner, r.URL.Path, body})
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(upstream.Close)
		return upstream
	}
	a, b := server("A"), server("B")
	t.Setenv("HA_BASE_URL", b.URL)
	t.Setenv("HA_TOKEN", "synthetic-B")
	sut.connectorRegistry = buildConnectorRegistry()
	sut.assetStore = assetstatus.NewStore(sut.assetStore, nil)
	windows := &homeAssistantMaintenanceWindows{}
	sut.groupMaintenanceStore = windows
	createHomeAssistantCredentialProfile(t, sut, "ha-action-a", "synthetic-A", a.URL)
	createHomeAssistantCredentialProfile(t, sut, "ha-action-b", "synthetic-B", b.URL)
	inventory := &stubHubCollectorStore{collectors: []hubcollector.Collector{
		{ID: "ha-a", CollectorType: "homeassistant", Enabled: true, AssetID: "ha-parent-a", Config: map[string]any{"base_url": a.URL, "credential_id": "ha-action-a"}},
		{ID: "ha-b", CollectorType: "homeassistant", Enabled: true, AssetID: "ha-parent-b", Config: map[string]any{"base_url": b.URL, "credential_id": "ha-action-b"}},
	}}
	sut.hubCollectorStore = inventory
	seed := func(id, source, kind, owner, entity, group string) {
		t.Helper()
		domain, _, _ := strings.Cut(entity, ".")
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Source: source, Type: kind, GroupID: group,
			Metadata: map[string]string{"collector_id": owner, "entity_id": entity, "domain": domain}}); err != nil {
			t.Fatal(err)
		}
	}
	seed("ha-parent-a", "homeassistant", "connector-cluster", "ha-a", "", "parent-a")
	seed("ha-parent-b", "homeassistant", "connector-cluster", "ha-b", "", "parent-b")
	targetA := assetid.ScopeCollectorAssetID("ha-entity-light-room_2", "ha-a")
	targetB := assetid.ScopeCollectorAssetID("ha-entity-light-room_2", "ha-b")
	seed(targetA, "homeassistant", "ha-entity", "ha-a", "light.room_2", "entity-a")
	seed(targetB, "home-assistant", "ha-entity", "ha-b", "light.room_2", "entity-b")
	// The maintenance owner sorts after a full page; a first-page-only guard
	// would incorrectly send the queued targetless service.
	for i := 0; i < 130; i++ {
		seed(fmt.Sprintf("ha-entity-sensor-%03d", i), "homeassistant", "ha-entity", "ha-a", fmt.Sprintf("sensor.row_%03d", i), "")
	}
	seed("zz-ha-entity-sibling", "home-assistant", "ha-entity", "ha-a", "light.sibling", "sibling-a")
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(effects) }
	wantEffect := func(before int, owner, path string, body map[string]string) {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		if len(effects) != before+1 {
			t.Fatalf("provider call count=%d want=%d", len(effects), before+1)
		}
		got := effects[len(effects)-1]
		if got.owner != owner || got.path != path || !reflect.DeepEqual(got.body, body) {
			t.Fatalf("effect=%+v want=%s %s %+v", got, owner, path, body)
		}
	}
	direct := func(action string, req connectorsdk.ActionRequest, status int) string {
		t.Helper()
		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, httptest.NewRequest(http.MethodPost,
			"/connectors/home-assistant/actions/"+action+"/execute", strings.NewReader(string(body))))
		if rec.Code != status {
			t.Fatalf("direct %s status=%d want=%d body=%s", action, rec.Code, status, rec.Body.String())
		}
		return rec.Body.String()
	}
	for _, tc := range []struct{ target, owner, expected string }{{targetA, "", "A"}, {"light.room_2", "ha-a", "A"}, {targetB, "", "B"}} {
		before := count()
		direct("automation.entity.toggle", connectorsdk.ActionRequest{TargetID: tc.target, Params: map[string]string{"collector_id": tc.owner}}, http.StatusOK)
		wantEffect(before, tc.expected, "/api/services/homeassistant/toggle", map[string]string{"entity_id": "light.room_2"})
	}
	params := map[string]string{"collector_id": "ha-a", "service": "light.turn_on", "entity_id": "light.payload", "area_id": "room",
		"device_id": "device", "floor_id": "floor", "label_id": "label", "brightness": "128", "requested_actor_label": "actor", "group_id": "audit"}
	before := count()
	direct("service.call", connectorsdk.ActionRequest{Params: params}, http.StatusOK)
	wire := map[string]string{"entity_id": "light.payload", "area_id": "room", "device_id": "device", "floor_id": "floor", "label_id": "label", "brightness": "128"}
	wantEffect(before, "A", "/api/services/light/turn_on", wire)
	before = count()
	direct("service.call", connectorsdk.ActionRequest{TargetID: targetA, Params: params}, http.StatusOK)
	wire["entity_id"] = "light.room_2"
	wantEffect(before, "A", "/api/services/light/turn_on", wire)
	before = count()
	direct("service.call", connectorsdk.ActionRequest{Params: map[string]string{"collector_id": "ha-a", "service": "homeassistant.reload_core_config"}}, http.StatusOK)
	wantEffect(before, "A", "/api/services/homeassistant/reload_core_config", map[string]string{})
	before = count()
	for _, req := range []connectorsdk.ActionRequest{
		{TargetID: targetA, Params: map[string]string{"collector_id": "ha-b"}},
		{TargetID: "light.room_2"},
		{TargetID: "ha-entity-missing", Params: map[string]string{"collector_id": "ha-a"}},
	} {
		direct("entity.toggle", req, http.StatusConflict)
	}
	direct("service.call", connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"collector_id": "ha-a", "service": "light.bad/path"}}, http.StatusConflict)
	direct("service.call", connectorsdk.ActionRequest{TargetID: targetA, Params: params, DryRun: true}, http.StatusOK)
	if count() != before {
		t.Fatal("invalid or dry-run action reached provider")
	}
	windows.blocked = "sibling-a"
	body := direct("service.call", connectorsdk.ActionRequest{TargetID: targetA, Params: params}, http.StatusLocked)
	if !strings.Contains(body, "all its known entities") || count() != before {
		t.Fatal("service maintenance scope missing or request dispatched")
	}
	// A narrow toggle does not affect the paused sibling; a service on B is
	// independent from that A-owned sibling despite identical entity names.
	direct("entity.toggle", connectorsdk.ActionRequest{TargetID: targetA}, http.StatusOK)
	wantEffect(before, "A", "/api/services/homeassistant/toggle", map[string]string{"entity_id": "light.room_2"})
	before = count()
	direct("service.call", connectorsdk.ActionRequest{TargetID: targetB, Params: map[string]string{"service": "light.turn_on"}}, http.StatusOK)
	wantEffect(before, "B", "/api/services/light/turn_on", map[string]string{"entity_id": "light.room_2"})
	windows.blocked = "parent-a"
	before = count()
	direct("entity.toggle", connectorsdk.ActionRequest{TargetID: "light.room_2", Params: map[string]string{"collector_id": "ha-a"}}, http.StatusLocked)
	if count() != before {
		t.Fatal("parent maintenance did not block raw toggle")
	}
	windows.blocked = "entity-a"
	direct("entity.toggle", connectorsdk.ActionRequest{TargetID: "light.room_2", Params: map[string]string{"collector_id": "ha-a"}}, http.StatusLocked)
	if count() != before {
		t.Fatal("raw toggle bypassed its stored entity maintenance")
	}
	windows.blocked = "parent-a"

	queue := &trueNASActionQueue{}
	deps := sut.buildActionsDeps()
	deps.JobQueue = queue
	enqueue := func(target string, params map[string]string, status int) {
		t.Helper()
		body, _ := json.Marshal(actions.ExecuteRequest{Type: actions.RunTypeConnectorAction, ConnectorID: "home-assistant", ActionID: "automation.service.call", Target: target, Params: params})
		rec := httptest.NewRecorder()
		deps.HandleActionExecute(rec, httptest.NewRequest(http.MethodPost, "/actions/execute", strings.NewReader(string(body))))
		if rec.Code != status {
			t.Fatalf("queue status=%d want=%d body=%s", rec.Code, status, rec.Body.String())
		}
	}
	enqueue(targetA, params, http.StatusLocked)
	if len(queue.jobs) != 0 || count() != before {
		t.Fatal("paused parent action was queued or sent")
	}
	windows.blocked = ""
	params["_ha_target_entity"] = "forged"
	enqueue(targetA, params, http.StatusAccepted)
	enqueue("", params, http.StatusAccepted)
	if len(queue.jobs) != 2 {
		t.Fatal("expected canonical and targetless queued jobs")
	}
	job, targetless := queue.jobs[0], queue.jobs[1]
	if job.Target != targetA || job.ActionID != "automation.service.call" || job.Params["collector_id"] != "ha-a" || job.Params["_ha_entity_id"] != "light.room_2" || job.Params["_ha_target_entity"] != "light.room_2" || targetless.Target != "" || targetless.Params["_ha_binding"] != "1" {
		t.Fatalf("queue binding/audit identity missing: %+v/%+v", job, targetless)
	}
	execute := func(ctx context.Context, job actions.Job, status string) actions.Result {
		t.Helper()
		result := sut.ensureProxmoxDeps().ExecuteActionInProcessContext(ctx, job)
		if result.Status != status {
			t.Fatalf("worker=%+v want=%s", result, status)
		}
		return result
	}
	windows.blocked = "sibling-a"
	result := execute(context.Background(), targetless, actions.StatusFailed)
	if !strings.Contains(result.Error, "all its known entities") || count() != before {
		t.Fatal("later-page maintenance did not stop queued targetless service with clear reason")
	}
	windows.blocked = ""
	execute(context.Background(), job, actions.StatusSucceeded)
	wantEffect(before, "A", "/api/services/light/turn_on", wire)
	before = count()
	unbound := job
	unbound.Params = maps.Clone(job.Params)
	delete(unbound.Params, "_ha_binding")
	if result := execute(context.Background(), unbound, actions.StatusFailed); len(result.Steps) != 1 || !strings.Contains(result.Steps[0].Error, "resubmit") {
		t.Fatalf("unbound job was not rejected: %+v", result)
	}
	seed(targetA, "homeassistant", "ha-entity", "ha-a", "light.changed", "entity-a")
	execute(context.Background(), job, actions.StatusFailed)
	if err := sut.assetStore.DeleteAsset(targetA); err != nil {
		t.Fatal(err)
	}
	execute(context.Background(), job, actions.StatusFailed)
	seed(targetA, "homeassistant", "ha-entity", "ha-a", "light.room_2", "entity-a")
	seed(targetA, "homeassistant", "ha-entity", "ha-b", "light.room_2", "entity-a")
	execute(context.Background(), job, actions.StatusFailed)
	seed(targetA, "homeassistant", "ha-entity", "ha-a", "light.room_2", "entity-a")
	inventory.collectors[0].Enabled = false
	execute(context.Background(), targetless, actions.StatusFailed)
	inventory.collectors[0].Enabled = true
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	windows.onRead = cancel
	execute(ctx, job, actions.StatusFailed)
	windows.onRead = nil
	if count() != before {
		t.Fatal("stale, unbound, disabled or canceled action reached provider")
	}
	// Neither a caller's params mutation nor the forged marker can rewrite a
	// previously bound targetless request. Its selector body remains intact.
	bound, err := sut.ensureHomeAssistantDeps().BindAction(context.Background(), "service.call", connectorsdk.ActionRequest{Params: params})
	if err != nil {
		t.Fatal(err)
	}
	params["collector_id"], params["area_id"] = "ha-b", "changed"
	call, err := sut.ensureHomeAssistantDeps().ExecuteAction(context.Background(), "service.call", bound)
	if err != nil || call.Status != "succeeded" {
		t.Fatalf("saved request snapshot failed: %+v/%v", call, err)
	}
	wire["entity_id"] = "light.payload"
	wantEffect(before, "A", "/api/services/light/turn_on", wire)
}
