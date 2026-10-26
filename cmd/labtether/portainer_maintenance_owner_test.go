package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/assetstatus"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
)

const portainerOwnerContainer = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type portainerOwnerFixture struct {
	t            *testing.T
	sut          *apiServer
	windows      *trueNASActionWindows
	collectors   *stubHubCollectorStore
	mu           sync.Mutex
	inspectID    string
	writes       []string
	foreignCalls int
	ids          map[string]string
}

func newPortainerOwnerFixture(t *testing.T) *portainerOwnerFixture {
	t.Helper()
	f := &portainerOwnerFixture{t: t, sut: newTestAPIServer(t), windows: &trueNASActionWindows{},
		inspectID: portainerOwnerContainer, ids: make(map[string]string)}
	allowInsecureTransportForConnectorTests(t)
	f.sut.assetStore = assetstatus.NewStore(f.sut.assetStore, nil)
	f.sut.groupMaintenanceStore = f.windows
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			f.writes = append(f.writes, r.Method+" "+r.URL.Path)
			if strings.HasSuffix(r.URL.Path, "/exec") {
				// Stop at the create boundary: this fixture proves authorization,
				// not a terminal WebSocket session.
				w.WriteHeader(http.StatusBadGateway)
			}
			_, _ = w.Write([]byte(`{}`))
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json"):
			_, _ = fmt.Fprintf(w, `{"Id":%q,"Name":"/friendly"}`, f.inspectID)
		case r.URL.Path == "/api/stacks":
			_, _ = w.Write([]byte(`[{"Id":8,"EndpointId":7},{"Id":9,"EndpointId":99}]`))
		case strings.HasSuffix(r.URL.Path, "/images/json"):
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected provider read: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.foreignCalls++
		f.mu.Unlock()
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(foreign.Close)
	ciphertext, err := f.sut.secretsManager.EncryptString("synthetic", "portainer-owner-credential")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID: "portainer-owner-credential", Name: "fixture", Kind: credentials.KindPortainerAPIKey,
		SecretCiphertext: ciphertext, Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	f.collectors = &stubHubCollectorStore{}
	for _, p := range []struct{ id, url string }{{"a", server.URL}, {"b", foreign.URL}} {
		collectorID := "portainer-" + p.id
		parent := "portainer-parent-" + p.id
		f.seed(parent, "host", "parent-"+p.id, nil)
		f.collectors.collectors = append(f.collectors.collectors, hubcollector.Collector{
			ID: collectorID, CollectorType: "portainer", Enabled: true, AssetID: parent,
			Config: map[string]any{"base_url": p.url, "credential_id": "portainer-owner-credential", "auth_method": "api_key"},
		})
		for _, resource := range []struct{ label, native, kind, key, value string }{
			{"loaded", "portainer-loaded", "container-host", "", ""},
			{"endpoint", "portainer-endpoint-7", "container-host", "", ""},
			{"container", "portainer-container-7-aaaaaaaaaaaa", "container", "container_id", portainerOwnerContainer},
			{"stack", "portainer-stack-8", "stack", "stack_id", "8"},
			{"sibling", "portainer-container-7-bbbbbbbbbbbb", "container", "container_id", strings.Repeat("b", 64)},
		} {
			id := assetid.ScopeCollectorAssetID(resource.native, collectorID)
			metadata := map[string]string{"collector_id": collectorID, "endpoint_id": "7"}
			if resource.key != "" {
				metadata[resource.key] = resource.value
			}
			f.seed(id, resource.kind, resource.label+"-"+p.id, metadata)
			f.ids[resource.label+"-"+p.id] = id
		}
	}
	f.sut.hubCollectorStore = f.collectors
	return f
}

func (f *portainerOwnerFixture) seed(id, kind, group string, metadata map[string]string) {
	f.t.Helper()
	if _, err := f.sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: id, Name: id, Type: kind, Source: "portainer", Status: "online", GroupID: group, Metadata: metadata,
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *portainerOwnerFixture) call(method, path, body string, allowed []string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, "/portainer/assets/"+f.ids["loaded-a"]+"/"+path, strings.NewReader(body))
	ctx := contextWithPrincipal(req.Context(), "owner", "owner")
	ctx = contextWithScopes(ctx, []string{"connectors:read", "connectors:write"})
	if allowed != nil {
		ctx = apiv2.ContextWithAllowedAssets(ctx, allowed)
	}
	req = req.WithContext(ctx)
	if strings.HasSuffix(path, "/exec") {
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
	}
	rec := httptest.NewRecorder()
	f.sut.handlePortainerAssets(rec, req)
	return rec
}

func (f *portainerOwnerFixture) effects() []string {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.foreignCalls != 0 {
		f.t.Fatalf("selected action contacted foreign provider %d times", f.foreignCalls)
	}
	return append([]string(nil), f.writes...)
}

// Each dedicated write must enforce its endpoint's maintenance window, while
// a paused sibling container must not disable unrelated writes.
func TestPortainerDedicatedMutationsHonorMaintenanceOwners(t *testing.T) {
	f := newPortainerOwnerFixture(t)
	for _, tc := range []struct{ method, path, body, effect string }{
		{"POST", "containers/friendly/start", "", "POST /api/endpoints/7/docker/containers/" + portainerOwnerContainer + "/start"},
		{"POST", "containers/friendly/stop", "", "POST /api/endpoints/7/docker/containers/" + portainerOwnerContainer + "/stop"},
		{"POST", "containers/friendly/restart", "", "POST /api/endpoints/7/docker/containers/" + portainerOwnerContainer + "/restart"},
		{"POST", "containers/friendly/kill", "", "POST /api/endpoints/7/docker/containers/" + portainerOwnerContainer + "/kill"},
		{"POST", "containers/friendly/remove", "", "DELETE /api/endpoints/7/docker/containers/" + portainerOwnerContainer},
		{"PUT", "stacks/8/compose", `{"content":"services: {}"}`, "PUT /api/stacks/8"},
		{"POST", "stacks/8/start", "", "POST /api/stacks/8/start"},
		{"POST", "stacks/8/stop", "", "POST /api/stacks/8/stop"},
		{"POST", "stacks/8/redeploy", "", "PUT /api/stacks/8/git/redeploy"},
		{"POST", "stacks/8/remove", "", "DELETE /api/stacks/8"},
		{"POST", "images/pull", `{"image":"fixture:tag"}`, "POST /api/endpoints/7/docker/images/create"},
		{"DELETE", "images/image-fixture", "", "DELETE /api/endpoints/7/docker/images/image-fixture"},
		{"POST", "volumes", `{"name":"fixture"}`, "POST /api/endpoints/7/docker/volumes/create"},
		{"DELETE", "volumes/fixture", "", "DELETE /api/endpoints/7/docker/volumes/fixture"},
		{"POST", "networks", `{"name":"fixture"}`, "POST /api/endpoints/7/docker/networks/create"},
		{"DELETE", "networks/fixture", "", "DELETE /api/endpoints/7/docker/networks/fixture"},
		{"GET", "containers/friendly/exec", "", "POST /api/endpoints/7/docker/containers/" + portainerOwnerContainer + "/exec"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			before := len(f.effects())
			f.windows.blocked = "endpoint-a"
			if rec := f.call(tc.method, tc.path, tc.body, nil); rec.Code != http.StatusLocked {
				t.Fatalf("paused endpoint: %d %s", rec.Code, rec.Body.String())
			}
			if len(f.effects()) != before {
				t.Fatal("paused endpoint still wrote to provider")
			}
			f.windows.blocked = "sibling-a"
			want := http.StatusOK
			if strings.HasSuffix(tc.path, "/exec") {
				want = http.StatusBadGateway
			}
			if rec := f.call(tc.method, tc.path, tc.body, nil); rec.Code != want {
				t.Fatalf("unrelated container blocked write: %d %s", rec.Code, rec.Body.String())
			}
			effects := f.effects()
			if len(effects) != before+1 || effects[before] != tc.effect {
				t.Fatalf("wrong effect: %v", effects[before:])
			}
		})
	}
	f.windows.blocked = "endpoint-a"
	if rec := f.call("GET", "images", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("maintenance blocked read: %d", rec.Code)
	}
	for _, group := range []string{"loaded-a", "parent-a", "container-a", "stack-a"} {
		f.windows.blocked = group
		path := "containers/friendly/stop"
		if group == "stack-a" {
			path = "stacks/8/stop"
		}
		before := len(f.effects())
		if rec := f.call("POST", path, "", nil); rec.Code != http.StatusLocked || len(f.effects()) != before {
			t.Fatalf("owner %s did not stop effect: %d %s", group, rec.Code, rec.Body.String())
		}
	}
}

func TestPortainerDedicatedAuthorizationUsesSelectedCollector(t *testing.T) {
	f := newPortainerOwnerFixture(t)
	allowed := []string{f.ids["loaded-a"], f.ids["container-a"]}
	if rec := f.call("POST", "containers/friendly/stop", "", allowed); rec.Code != http.StatusOK {
		t.Fatalf("foreign same-ID row denied selected owner: %d %s", rec.Code, rec.Body.String())
	}
	for _, key := range []string{"endpoint-a", "stack-a", "sibling-a"} {
		allowed = append(allowed, f.ids[key])
	}
	if rec := f.call("GET", "images", "", allowed); rec.Code != http.StatusOK {
		t.Fatalf("foreign endpoint contaminated access: %d", rec.Code)
	}
	if err := f.sut.assetStore.DeleteAsset(f.ids["container-a"]); err != nil {
		t.Fatal(err)
	}
	before := len(f.effects())
	if rec := f.call("POST", "containers/friendly/stop", "", []string{f.ids["loaded-a"], f.ids["container-b"]}); rec.Code != http.StatusForbidden || len(f.effects()) != before {
		t.Fatalf("foreign row authorized selected provider: %d %s", rec.Code, rec.Body.String())
	}
	legacy := "portainer-container-7-aaaaaaaaaaaa"
	f.seed(legacy, "container", "legacy", map[string]string{"endpoint_id": "7", "container_id": portainerOwnerContainer})
	allowed = []string{f.ids["loaded-a"], legacy}
	if rec := f.call("POST", "containers/friendly/stop", "", allowed); rec.Code != http.StatusForbidden || len(f.effects()) != before {
		t.Fatalf("ambiguous unowned row authorized effect: %d", rec.Code)
	}
	f.collectors.collectors = f.collectors.collectors[:1]
	if rec := f.call("POST", "containers/friendly/stop", "", allowed); rec.Code != http.StatusOK || len(f.effects()) != before+1 {
		t.Fatalf("unique legacy owner rejected: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPortainerDedicatedMutationRejectsAmbiguousNativeIdentity(t *testing.T) {
	f := newPortainerOwnerFixture(t)
	f.seed(f.ids["container-a"], "container", "container-a", map[string]string{
		"collector_id": "portainer-a", "endpoint_id": "7", "container_id": strings.Repeat("a", 12) + strings.Repeat("c", 52),
	})
	if rec := f.call("POST", "containers/aaaaaaaaaaaa/stop", "", nil); rec.Code != http.StatusConflict || len(f.effects()) != 0 {
		t.Fatalf("12-character collision reached provider mutation: %d %s", rec.Code, rec.Body.String())
	}
	f.seed(f.ids["container-a"], "container", "container-a", map[string]string{
		"collector_id": "portainer-b", "endpoint_id": "7", "container_id": portainerOwnerContainer,
	})
	if rec := f.call("POST", "containers/friendly/stop", "", nil); rec.Code != http.StatusConflict || len(f.effects()) != 0 {
		t.Fatalf("scope and metadata conflict reached mutation: %d %s", rec.Code, rec.Body.String())
	}
	f.mu.Lock()
	f.inspectID = "short-id"
	f.mu.Unlock()
	if rec := f.call("POST", "containers/friendly/stop", "", nil); rec.Code != http.StatusConflict || len(f.effects()) != 0 {
		t.Fatalf("invalid inspect identity reached mutation: %d", rec.Code)
	}
	if rec := f.call("POST", "stacks/9/stop", "", nil); rec.Code != http.StatusNotFound || len(f.effects()) != 0 {
		t.Fatalf("stack from another endpoint reached mutation: %d", rec.Code)
	}
}
