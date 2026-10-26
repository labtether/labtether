package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestHomeAssistantRuntimeKeepsExactOwnerBeyondInventoryPage(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	var callsA, callsB atomic.Int32
	serve := func(token string, calls *atomic.Int32) *httptest.Server {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			var body struct {
				EntityID string `json:"entity_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if r.Method != http.MethodPost || r.URL.Path != "/api/services/light/turn_on" || body.EntityID != "light.qa" || r.Header.Get("Authorization") != "Bearer "+token {
				t.Errorf("wrong Home Assistant effect: %s %s entity=%s", r.Method, r.URL.Path, body.EntityID)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(server.Close)
		return server
	}
	a, b := serve("synthetic-owner-a", &callsA), serve("synthetic-owner-b", &callsB)
	createHomeAssistantCredentialProfile(t, sut, "ha-owner-a", "synthetic-owner-a", a.URL)
	createHomeAssistantCredentialProfile(t, sut, "ha-owner-b", "synthetic-owner-b", b.URL)
	collectorA := hubcollector.Collector{ID: "ha-a", CollectorType: "homeassistant", Enabled: true,
		Config: map[string]any{"base_url": a.URL, "credential_id": "ha-owner-a"}}
	collectorB := hubcollector.Collector{ID: "ha-b", CollectorType: "homeassistant", Enabled: true,
		Config: map[string]any{"base_url": b.URL, "credential_id": "ha-owner-b"}}
	var unrelated []hubcollector.Collector
	for i := 0; i < 201; i++ {
		unrelated = append(unrelated, hubcollector.Collector{ID: fmt.Sprintf("unrelated-%03d", i), CollectorType: "docker", Enabled: true})
	}
	store := &runtimeOwnerCollectorStore{stubHubCollectorStore: &stubHubCollectorStore{}}
	sut.hubCollectorStore = store
	seed := func(id, owner string) {
		t.Helper()
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id, Source: "home-assistant", Type: "ha-entity",
			Metadata: map[string]string{"entity_id": "light.qa", "domain": "light", "collector_id": owner},
		}); err != nil {
			t.Fatal(err)
		}
	}
	invoke := func(id, owner string, want int, aDelta, bDelta int32) {
		t.Helper()
		beforeA, beforeB := callsA.Load(), callsB.Load()
		body, err := json.Marshal(map[string]string{"action": "turn_on", "collector_id": owner})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/api/v2/homeassistant/entities/"+id, strings.NewReader(string(body)))
		ctx := contextWithPrincipal(r.Context(), "ha-owner", "owner")
		r = r.WithContext(contextWithScopes(ctx, []string{"homeassistant:write"}))
		w := httptest.NewRecorder()
		sut.handleV2HAEntityActions(w, r)
		if w.Code != want || callsA.Load() != beforeA+aDelta || callsB.Load() != beforeB+bDelta {
			t.Fatalf("target=%s owner=%s status=%d calls=%d/%d body=%s", id, owner, w.Code, callsA.Load()-beforeA, callsB.Load()-beforeB, w.Body.String())
		}
		if store.listCalls != 0 {
			t.Fatal("runtime used a truncated global collector page")
		}
	}
	// Named and unique legacy owners remain reachable beyond the first 200
	// unrelated collectors, through the actual HTTP bridge and provider client.
	store.collectors = append(append([]hubcollector.Collector{}, unrelated...), collectorA, collectorB)
	named := assetid.ScopeCollectorAssetID("ha-entity-light-qa", collectorA.ID)
	seed(named, collectorA.ID)
	invoke(named, "", http.StatusOK, 1, 0)
	invoke(named, collectorB.ID, http.StatusConflict, 0, 0)
	conflict := assetid.ScopeCollectorAssetID("ha-entity-light-wrong", collectorB.ID)
	seed(conflict, collectorA.ID)
	invoke(conflict, "", http.StatusConflict, 0, 0)
	store.collectors = append(append([]hubcollector.Collector{}, unrelated...), collectorB)
	seed("ha-legacy", "")
	invoke("ha-legacy", "", http.StatusOK, 0, 1)
	invoke("ha-legacy", collectorB.ID, http.StatusOK, 0, 1)
	scoped := assetid.ScopeCollectorAssetID("ha-entity-light-scoped", collectorB.ID)
	seed(scoped, "")
	invoke(scoped, "", http.StatusOK, 0, 1)
	// A visible A plus a hidden B is not a unique legacy owner. A client
	// parameter cannot resolve an otherwise unowned legacy row's ambiguity.
	store.collectors = append([]hubcollector.Collector{collectorA}, unrelated...)
	store.collectors = append(store.collectors, collectorB)
	invoke("ha-legacy", "", http.StatusConflict, 0, 0)
	invoke("ha-legacy", collectorA.ID, http.StatusConflict, 0, 0)
	invoke(scoped, collectorA.ID, http.StatusConflict, 0, 0)
	invoke(scoped, collectorB.ID, http.StatusOK, 0, 1)
	for _, problem := range []string{"missing", "disabled", "wrong type", "invalid config"} {
		broken := collectorA
		store.collectors = []hubcollector.Collector{collectorB}
		switch problem {
		case "disabled":
			broken.Enabled = false
		case "wrong type":
			broken.CollectorType = "docker"
		case "invalid config":
			broken.Config = map[string]any{"base_url": a.URL, "credential_id": "missing"}
		}
		if problem != "missing" {
			store.collectors = append(store.collectors, broken)
		}
		invoke(named, "", http.StatusConflict, 0, 0)
		// B is still a valid positive control, not a broken fallback transport.
		invoke(scoped, collectorB.ID, http.StatusOK, 0, 1)
	}
}
