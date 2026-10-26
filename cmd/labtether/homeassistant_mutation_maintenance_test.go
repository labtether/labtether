package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

type homeAssistantMaintenanceWindows struct {
	persistence.GroupMaintenanceStore
	blocked string
	err     error
	onRead  func()
}

func (s *homeAssistantMaintenanceWindows) ListGroupMaintenanceWindows(group string, _ *time.Time, _ int) ([]groupmaintenance.MaintenanceWindow, error) {
	if s.onRead != nil {
		s.onRead()
	}
	if s.err != nil {
		return nil, s.err
	}
	if group == s.blocked {
		return []groupmaintenance.MaintenanceWindow{{ID: "ha-window", GroupID: group, BlockActions: true}}, nil
	}
	return nil, nil
}

type homeAssistantMaintenanceCollectors struct {
	*stubHubCollectorStore
	afterGet  func()
	listLimit int
}

func (s *homeAssistantMaintenanceCollectors) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	row, found, err := s.stubHubCollectorStore.GetHubCollector(id)
	if s.afterGet != nil {
		action := s.afterGet
		s.afterGet = nil
		action()
	}
	return row, found, err
}

func (s *homeAssistantMaintenanceCollectors) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	rows, err := s.stubHubCollectorStore.ListHubCollectors(limit, enabledOnly)
	if s.listLimit > 0 && len(rows) > s.listLimit {
		rows = rows[:s.listLimit]
	}
	return rows, err
}

func TestHomeAssistantWritesHonorEntityAndCollectorMaintenance(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	windows := &homeAssistantMaintenanceWindows{}
	sut.groupMaintenanceStore = windows
	var mu sync.Mutex
	var writes []string
	var parameters []map[string]any
	server := func(owner string) *httptest.Server {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			entityID, _ := body["entity_id"].(string)
			if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/services/") || entityID == "" {
				t.Errorf("unexpected Home Assistant request: %s %s", r.Method, r.URL.Path)
			}
			mu.Lock()
			writes = append(writes, owner+" "+r.URL.Path+" "+entityID)
			parameters = append(parameters, body)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(upstream.Close)
		return upstream
	}
	a, b := server("a"), server("b")
	createHomeAssistantCredentialProfile(t, sut, "ha-maintenance-a", "synthetic-a", a.URL)
	createHomeAssistantCredentialProfile(t, sut, "ha-maintenance-b", "synthetic-b", b.URL)
	collectors := &homeAssistantMaintenanceCollectors{stubHubCollectorStore: &stubHubCollectorStore{collectors: []hubcollector.Collector{
		{ID: "ha-a", CollectorType: "homeassistant", Enabled: true, AssetID: "ha-parent-a", Config: map[string]any{"base_url": a.URL, "credential_id": "ha-maintenance-a"}},
		{ID: "ha-b", CollectorType: "homeassistant", Enabled: true, AssetID: "ha-parent-b", Config: map[string]any{"base_url": b.URL, "credential_id": "ha-maintenance-b"}},
	}}}
	sut.hubCollectorStore = collectors
	seed := func(id, group, entity, domain, owner string) {
		t.Helper()
		_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id, Type: "ha-entity", Source: "home-assistant", GroupID: group,
			Metadata: map[string]string{"entity_id": entity, "domain": domain, "collector_id": owner},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	seed("ha-parent-a", "parent-a", "", "", "")
	seed("ha-parent-b", "parent-b", "", "", "")
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(writes) }
	request := func(ctx context.Context, method, id, body string, collection bool) *httptest.ResponseRecorder {
		path := "/api/v2/homeassistant/entities/" + id
		if collection {
			path = "/api/v2/homeassistant/entities"
		}
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if ctx == nil {
			ctx = r.Context()
		}
		ctx = contextWithPrincipal(ctx, "ha-owner", "owner")
		ctx = contextWithScopes(ctx, []string{"homeassistant:read", "homeassistant:write"})
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()
		if collection {
			sut.handleV2HAEntities(w, r)
		} else {
			sut.handleV2HAEntityActions(w, r)
		}
		return w
	}
	for _, domain := range []string{"light", "automation", "scene"} {
		id := assetid.ScopeCollectorAssetID("ha-entity-"+domain+"-qa", "ha-a")
		seed(id, domain, domain+".qa", domain, "ha-a")
		body := `{"asset_id":"` + id + `","action":"turn_on"}`
		if domain == "light" {
			body = `{"asset_id":"` + id + `","action":"turn_on","params":{"entity_id":"light.sibling","brightness":"128"}}`
		}
		before := count()
		for _, group := range []string{domain, "parent-a"} {
			windows.blocked = group
			w := request(nil, http.MethodPost, id, body, domain == "light")
			if w.Code != http.StatusLocked || count() != before || !strings.Contains(w.Body.String(), `"error":"maintenance_blocked"`) {
				t.Fatalf("%s paused %s: status=%d writes=%d body=%s", domain, group, w.Code, count()-before, w.Body.String())
			}
			if w := request(nil, http.MethodGet, id, "", false); w.Code != http.StatusOK || count() != before {
				t.Fatalf("paused read changed: status=%d body=%s", w.Code, w.Body.String())
			}
		}
		windows.blocked = "parent-b"
		w := request(nil, http.MethodPost, id, body, domain == "light")
		if w.Code != http.StatusOK || count() != before+1 {
			t.Fatalf("foreign parent blocked %s: %d %s", domain, w.Code, w.Body.String())
		}
		mu.Lock()
		actual := writes[len(writes)-1]
		actualParams := parameters[len(parameters)-1]
		mu.Unlock()
		if domain == "light" && (actualParams["entity_id"] != "light.qa" || actualParams["brightness"] != "128") {
			t.Fatalf("entity override or normal value changed: %+v", actualParams)
		}
		if actual != "a /api/services/"+domain+"/turn_on "+domain+".qa" {
			t.Fatalf("wrong effect: %s", actual)
		}
	}
	id := assetid.ScopeCollectorAssetID("ha-entity-light-qa", "ha-a")
	body := `{"action":"turn_on"}`
	before := count()
	seed(assetid.ScopeCollectorAssetID("ha-entity-light-sibling", "ha-a"), "sibling", "light.sibling", "light", "ha-a")
	windows.blocked = "sibling"
	for _, key := range []string{"area_id", "device_id", "floor_id", "label_id", " area_id "} {
		payload, err := json.Marshal(map[string]any{"action": "turn_on", "params": map[string]string{key: "sibling-target"}})
		if err != nil {
			t.Fatal(err)
		}
		if w := request(nil, http.MethodPost, id, string(payload), false); w.Code != http.StatusBadRequest || count() != before {
			t.Fatalf("additional selector %q reached provider: %d %s", key, w.Code, w.Body.String())
		}
	}
	windows.blocked = ""
	windows.err = errors.New("private database failure")
	if w := request(nil, http.MethodPost, id, body, false); w.Code != http.StatusInternalServerError || count() != before || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("maintenance lookup failure: %d %s", w.Code, w.Body.String())
	}
	windows.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	windows.onRead = cancel
	if w := request(ctx, http.MethodPost, id, body, false); w.Code != http.StatusRequestTimeout || count() != before {
		t.Fatalf("canceled guard dispatched: %d %s", w.Code, w.Body.String())
	}
	windows.onRead = nil
	collectors.afterGet = func() { seed(id, "light", "light.changed", "light", "ha-a") }
	if w := request(nil, http.MethodPost, id, body, false); w.Code != http.StatusConflict || count() != before {
		t.Fatalf("changed entity dispatched: %d %s", w.Code, w.Body.String())
	}
	seed(id, "light", "light.qa", "light", "ha-a")
	collectors.afterGet = func() { collectors.collectors[0].Enabled = false }
	if w := request(nil, http.MethodPost, id, body, false); w.Code != http.StatusConflict || count() != before {
		t.Fatalf("retired collector dispatched: %d %s", w.Code, w.Body.String())
	}
	collectors.collectors[0].Enabled = true
	conflict := assetid.ScopeCollectorAssetID("ha-entity-light-conflict", "ha-b")
	seed(conflict, "light", "light.qa", "light", "ha-a")
	if w := request(nil, http.MethodPost, conflict, body, false); w.Code != http.StatusConflict || count() != before {
		t.Fatalf("conflicting namespace dispatched: %d %s", w.Code, w.Body.String())
	}
	// The old runtime's truncated inventory must not establish uniqueness for
	// an unowned legacy row. The guard checks the complete bounded type lookup.
	seed("ha-legacy", "light", "light.qa", "light", "")
	collectors.listLimit = 1
	if w := request(nil, http.MethodPost, "ha-legacy", body, false); w.Code != http.StatusConflict || count() != before {
		t.Fatalf("false singleton dispatched: %d %s", w.Code, w.Body.String())
	}
	collectors.listLimit = 0
	other := assetid.ScopeCollectorAssetID("ha-entity-light-qa", "ha-b")
	seed(other, "foreign-light", "light.qa", "light", "ha-b")
	windows.blocked = "parent-a"
	if w := request(nil, http.MethodPost, other, body, false); w.Code != http.StatusOK || count() != before+1 {
		t.Fatalf("selected second provider failed: %d %s", w.Code, w.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if writes[len(writes)-1] != "b /api/services/light/turn_on light.qa" {
		t.Fatalf("second provider effect: %s", writes[len(writes)-1])
	}
}
