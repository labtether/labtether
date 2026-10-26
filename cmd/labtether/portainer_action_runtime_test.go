package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/connectorsdk"
)

func portainerDirectAction(t *testing.T, f *portainerOwnerFixture, actionID string, req connectorsdk.ActionRequest, want int) {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/connectors/portainer/actions/"+actionID+"/execute", strings.NewReader(string(body)))
	r = r.WithContext(contextWithPrincipal(r.Context(), "owner", "owner"))
	rec := httptest.NewRecorder()
	f.sut.ensureCollectorsDeps().HandleConnectorActions(rec, r)
	if rec.Code != want {
		t.Fatalf("direct: status=%d want=%d body=%s", rec.Code, want, rec.Body.String())
	}
}

func portainerQueueAction(t *testing.T, f *portainerOwnerFixture, actionID, target string, params map[string]string, allowed []string, want int) []actions.Job {
	t.Helper()
	queue := &trueNASActionQueue{}
	d := f.sut.buildActionsDeps()
	d.JobQueue = queue
	body, err := json.Marshal(actions.ExecuteRequest{Type: actions.RunTypeConnectorAction, ConnectorID: "portainer", ActionID: actionID, Target: target, Params: params})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/actions/execute", strings.NewReader(string(body)))
	ctx := contextWithPrincipal(r.Context(), "owner", "owner")
	ctx = contextWithScopes(ctx, []string{"actions:exec"})
	if allowed != nil {
		ctx = apiv2.ContextWithAllowedAssets(ctx, allowed)
	}
	r = r.WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleActionExecute(rec, r)
	if rec.Code != want {
		t.Fatalf("queue: status=%d want=%d body=%s", rec.Code, want, rec.Body.String())
	}
	if want == http.StatusAccepted && len(queue.jobs) != 1 {
		t.Fatalf("accepted without exact queued job: %v", queue.jobs)
	}
	if want != http.StatusAccepted && len(queue.jobs) != 0 {
		t.Fatal("rejected action still queued")
	}
	return queue.jobs
}

func TestPortainerConfiguredActionPinsProviderAndContainer(t *testing.T) {
	f := newPortainerOwnerFixture(t)
	f.sut.connectorRegistry = buildConnectorRegistry() // Startup catalog, not a test-only configured registry.
	portainerDirectAction(t, f, "workload.stop", connectorsdk.ActionRequest{TargetID: f.ids["container-a"]}, http.StatusOK)
	wantEffect := "POST /api/endpoints/7/docker/containers/" + portainerOwnerContainer + "/stop"
	if effects := f.effects(); len(effects) != 1 || effects[0] != wantEffect {
		t.Fatalf("direct used wrong target: %v", effects)
	}
	params := map[string]string{"collector_id": "portainer-a", "portainer_asset_id": "forged"}
	target := "portainer-container-7-friendly"
	job := portainerQueueAction(t, f, "container.stop", target, params, nil, http.StatusAccepted)[0]
	if job.Target != target || job.Params["collector_id"] != "portainer-a" || job.Params["endpoint_id"] != "7" || job.Params["container_id"] != portainerOwnerContainer {
		t.Fatalf("queue did not pin exact provider/resource: %+v", job)
	}
	if marker, present := job.Params["portainer_asset_id"]; !present || marker != "" {
		t.Fatal("raw binding did not replace the caller's private marker")
	}
	// The name now identifies the other full container. Execution must consume
	// the saved ID and must not inspect/rebind either the old name or short ID.
	f.mu.Lock()
	f.inspectID = strings.Repeat("b", 64)
	f.mu.Unlock()
	f.windows.blocked = "container-a"
	before := len(f.effects())
	worker := f.sut.ensureProxmoxDeps()
	if result := worker.ExecuteActionInProcessContext(context.Background(), job); result.Status != actions.StatusFailed || !strings.Contains(result.Error, "maintenance") || len(f.effects()) != before {
		t.Fatalf("queued maintenance owner bypassed: %+v", result)
	}
	f.windows.blocked = ""
	if result := worker.ExecuteActionInProcessContext(context.Background(), job); result.Status != actions.StatusSucceeded {
		t.Fatalf("bound worker failed: %+v", result)
	}
	if effects := f.effects(); len(effects) != before+1 || effects[before] != wantEffect {
		t.Fatalf("queued alias changed effect: %v", effects[before:])
	}
	f.collectors.collectors[0].Enabled = false
	before = len(f.effects())
	if result := worker.ExecuteActionInProcessContext(context.Background(), job); result.Status != actions.StatusFailed || len(f.effects()) != before {
		t.Fatalf("disabled owner fell back: %+v", result)
	}
	f.collectors.collectors[0].Enabled = true
	unbound := job
	unbound.Params = nil
	if result := worker.ExecuteActionInProcessContext(context.Background(), unbound); result.Status != actions.StatusFailed || !strings.Contains(result.Error, "resubmit") || len(f.effects()) != before {
		t.Fatalf("old unbound job dispatched: %+v", result)
	}
}

func TestPortainerConfiguredStacksAndScopedAuthorization(t *testing.T) {
	f := newPortainerOwnerFixture(t)
	f.sut.connectorRegistry = buildConnectorRegistry()
	// The real discovery catalog must tell raw stack callers about the
	// endpoint argument that the provider has always required.
	rec := httptest.NewRecorder()
	f.sut.ensureCollectorsDeps().HandleConnectorActions(rec, httptest.NewRequest("GET", "/connectors/portainer/actions", nil))
	var listing struct {
		Actions []connectorsdk.ActionDescriptor `json:"actions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("descriptor response: %d %s", rec.Code, rec.Body.String())
	}
	stacks := 0
	for _, action := range listing.Actions {
		if !strings.HasPrefix(action.ID, "stack.") {
			continue
		}
		stacks++
		found := false
		for _, param := range action.Parameters {
			if param.Key == "endpoint_id" && param.Required {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s omits its required endpoint", action.ID)
		}
	}
	if stacks != 4 {
		t.Fatalf("expected four stack descriptors, got%d", stacks)
	}
	raw := connectorsdk.ActionRequest{TargetID: "portainer-stack-8", Params: map[string]string{"collector_id": "portainer-a", "endpoint_id": "7"}}
	portainerDirectAction(t, f, "stack.stop", raw, http.StatusOK)
	before := len(f.effects())
	dryRun := raw
	dryRun.DryRun = true
	portainerDirectAction(t, f, "stack.stop", dryRun, http.StatusOK)
	if len(f.effects()) != before {
		t.Fatal("stack dry run performed a write")
	}
	bad := connectorsdk.ActionRequest{TargetID: raw.TargetID, Params: map[string]string{"collector_id": "portainer-a", "endpoint_id": "99"}}
	portainerDirectAction(t, f, "stack.stop", bad, http.StatusConflict)
	delete(bad.Params, "endpoint_id")
	portainerDirectAction(t, f, "stack.stop", bad, http.StatusConflict)
	if len(f.effects()) != before {
		t.Fatal("wrong/missing stack endpoint wrote to provider")
	}
	target := f.ids["stack-a"]
	job := portainerQueueAction(t, f, "stack.stop", target, nil, []string{target}, http.StatusAccepted)[0]
	if job.Target != target || job.Params["stack_id"] != "8" || job.Params["endpoint_id"] != "7" || job.Params["portainer_asset_id"] != target {
		t.Fatalf("canonical stack not bound: %+v", job)
	}
	matching := map[string]string{"collector_id": "portainer-a", "endpoint_id": "7"}
	matched := portainerQueueAction(t, f, "stack.stop", target, matching, []string{target}, http.StatusAccepted)[0]
	if matched.Target != target || matched.Params["collector_id"] != "portainer-a" || matched.Params["endpoint_id"] != "7" || matched.Params["stack_id"] != "8" || matched.Params["portainer_asset_id"] != target {
		t.Fatalf("matching scoped parameters lost their stored owner: %+v", matched)
	}
	for _, params := range []map[string]string{{"endpoint_id": "99"}, {"collector_id": "portainer-b"}} {
		portainerQueueAction(t, f, "stack.stop", target, params, []string{target}, http.StatusForbidden)
	}
	portainerQueueAction(t, f, "stack.stop", raw.TargetID, raw.Params, []string{raw.TargetID}, http.StatusForbidden)
	f.windows.blocked = "stack-a"
	worker := f.sut.ensureProxmoxDeps()
	if result := worker.ExecuteActionInProcessContext(context.Background(), job); result.Status != actions.StatusFailed || len(f.effects()) != before {
		t.Fatalf("queued stack maintenance bypassed: %+v", result)
	}
	f.windows.blocked = ""
	if result := worker.ExecuteActionInProcessContext(context.Background(), job); result.Status != actions.StatusSucceeded || len(f.effects()) != before+1 {
		t.Fatalf("canonical stack execution failed: %+v", result)
	}
}

func TestPortainerConfiguredBindingRejectsInventoryDrift(t *testing.T) {
	f := newPortainerOwnerFixture(t)
	f.sut.connectorRegistry = buildConnectorRegistry()
	target := f.ids["container-a"]
	job := portainerQueueAction(t, f, "workload.stop", target, nil, []string{target}, http.StatusAccepted)[0]
	if job.Params["portainer_asset_id"] != target {
		t.Fatal("canonical target marker missing")
	}
	worker := f.sut.ensureProxmoxDeps()
	if err := f.sut.assetStore.DeleteAsset(target); err != nil {
		t.Fatal(err)
	}
	if result := worker.ExecuteActionInProcessContext(context.Background(), job); result.Status != actions.StatusFailed || len(f.effects()) != 0 {
		t.Fatalf("removed asset treated as raw alias: %+v", result)
	}
	f.seed(target, "container", "container-a", map[string]string{"collector_id": "portainer-a", "endpoint_id": "7", "container_id": strings.Repeat("b", 64)})
	if result := worker.ExecuteActionInProcessContext(context.Background(), job); result.Status != actions.StatusFailed || len(f.effects()) != 0 {
		t.Fatalf("changed canonical container followed: %+v", result)
	}
	// A canonical row claiming B must fail if selected-provider inspect returns A.
	portainerDirectAction(t, f, "container.stop", connectorsdk.ActionRequest{TargetID: target}, http.StatusConflict)
	// Even when the provider agrees with changed metadata, a fresh restricted
	// request cannot use the old asset ID to reach a different container/endpoint.
	for _, changed := range []struct{ endpoint, container string }{{"7", strings.Repeat("b", 64)}, {"99", portainerOwnerContainer}} {
		f.seed(target, "container", "container-a", map[string]string{"collector_id": "portainer-a", "endpoint_id": changed.endpoint, "container_id": changed.container})
		f.mu.Lock()
		f.inspectID = changed.container
		f.mu.Unlock()
		portainerQueueAction(t, f, "container.stop", target, nil, []string{target}, http.StatusConflict)
		if len(f.effects()) != 0 {
			t.Fatal("new request followed canonical asset metadata drift")
		}
	}
	if err := f.sut.assetStore.DeleteAsset(target); err != nil {
		t.Fatal(err)
	}
	raw := "portainer-container-7-friendly"
	job = portainerQueueAction(t, f, "container.stop", raw, map[string]string{"collector_id": "portainer-a"}, nil, http.StatusAccepted)[0]
	f.seed(raw, "container", "new-owner", map[string]string{"collector_id": "portainer-a", "endpoint_id": "7", "container_id": portainerOwnerContainer})
	if result := worker.ExecuteActionInProcessContext(context.Background(), job); result.Status != actions.StatusFailed || len(f.effects()) != 0 {
		t.Fatalf("raw target adopted a new asset: %+v", result)
	}
}
