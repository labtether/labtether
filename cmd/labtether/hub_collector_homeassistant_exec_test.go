package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestExecuteTwoHomeAssistantCollectorsKeepRepeatedEntityIDDistinct(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/states", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"entity_id":"sun.sun","state":"above_horizon","attributes":{"friendly_name":"Sun"}}]`))
	})
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"2026.7.0","location_name":"Test Home"}`))
	})
	mux.HandleFunc("/api/supervisor/stats", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	collectorIDs := []string{"collector-ha-simba", "collector-ha-mccann"}
	for index, collectorID := range collectorIDs {
		credentialID := "cred-ha-scope-" + string(rune('a'+index))
		createHomeAssistantCredentialProfile(t, sut, credentialID, "ha-token", server.URL)
		collector := hubcollector.Collector{
			ID:            collectorID,
			AssetID:       "ha-cluster-" + collectorID,
			CollectorType: hubcollector.CollectorTypeHomeAssistant,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": credentialID,
			},
		}
		store.statusByID[collector.ID] = collector
		sut.executeCollector(context.Background(), collector)
	}

	for _, collectorID := range collectorIDs {
		id := assetid.ScopeCollectorAssetID("ha-entity-sun-sun", collectorID)
		asset, ok, err := sut.assetStore.GetAsset(id)
		if err != nil || !ok {
			t.Fatalf("expected scoped entity %s: ok=%v err=%v", id, ok, err)
		}
		if asset.Metadata["collector_id"] != collectorID || asset.Metadata["entity_id"] != "sun.sun" {
			t.Fatalf("unexpected entity metadata for %s: %#v", id, asset.Metadata)
		}
	}
	if _, ok, err := sut.assetStore.GetAsset("ha-entity-sun-sun"); err != nil || ok {
		t.Fatalf("unexpected ambiguous legacy entity: ok=%v err=%v", ok, err)
	}
}
