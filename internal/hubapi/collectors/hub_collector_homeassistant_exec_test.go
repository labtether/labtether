package collectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

type homeAssistantCollectorStatusStore struct {
	collector hubcollector.Collector
}

func (s *homeAssistantCollectorStatusStore) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	s.collector = hubcollector.Collector{
		ID:              "collector-ha",
		AssetID:         req.AssetID,
		CollectorType:   req.CollectorType,
		Config:          req.Config,
		Enabled:         true,
		IntervalSeconds: hubcollector.DefaultIntervalSeconds,
	}
	return s.collector, nil
}

func (s *homeAssistantCollectorStatusStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	if s.collector.ID != id {
		return hubcollector.Collector{}, false, nil
	}
	return s.collector, true, nil
}

func (s *homeAssistantCollectorStatusStore) ListHubCollectors(_ int, enabledOnly bool) ([]hubcollector.Collector, error) {
	if s.collector.ID == "" || (enabledOnly && !s.collector.Enabled) {
		return nil, nil
	}
	return []hubcollector.Collector{s.collector}, nil
}

func (s *homeAssistantCollectorStatusStore) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	if s.collector.ID != id {
		return hubcollector.Collector{}, hubcollector.ErrCollectorNotFound
	}
	if req.Config != nil {
		s.collector.Config = *req.Config
	}
	if req.Enabled != nil {
		s.collector.Enabled = *req.Enabled
	}
	if req.IntervalSeconds != nil {
		s.collector.IntervalSeconds = *req.IntervalSeconds
	}
	return s.collector, nil
}

func (s *homeAssistantCollectorStatusStore) DeleteHubCollector(id string) error {
	if s.collector.ID != id {
		return hubcollector.ErrCollectorNotFound
	}
	s.collector = hubcollector.Collector{}
	return nil
}

func (s *homeAssistantCollectorStatusStore) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	if s.collector.ID != id {
		return hubcollector.ErrCollectorNotFound
	}
	s.collector.LastStatus = status
	s.collector.LastError = lastError
	collectedAt = collectedAt.UTC()
	s.collector.LastCollectedAt = &collectedAt
	return nil
}

func TestHomeAssistantCollectorOutagePreservesInventoryAndRecoveryRestoresParentOnline(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	available := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !available {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/states":
			_, _ = w.Write([]byte(`[{
				"entity_id":"light.recovered",
				"state":"on",
				"attributes":{"friendly_name":"Recovered Light"},
				"last_changed":"2026-07-16T00:00:00Z",
				"last_updated":"2026-07-16T00:00:00Z"
			}]`))
		case "/api/config":
			_, _ = w.Write([]byte(`{"version":"2026.7.0","location_name":"QA Home"}`))
		case "/api/hassio/core/stats":
			http.Error(w, "supervisor unavailable", http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	assetStore := persistence.NewMemoryAssetStore()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "ha-root",
		Type:    "connector-cluster",
		Name:    "Disposable HA",
		Source:  "homeassistant",
		Status:  "online",
		Metadata: map[string]string{
			"collector_id":       "collector-ha",
			"collector_base_url": server.URL,
			"connector_type":     "homeassistant",
			"discovered":         "7",
			"stale_marker":       "preserve-me",
		},
	}); err != nil {
		t.Fatalf("seed Home Assistant root: %v", err)
	}
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "ha-entity-light-last-success",
		Type:    "ha-entity",
		Name:    "Last Successful Light",
		Source:  "homeassistant",
		Status:  "online",
		Metadata: map[string]string{
			"collector_id": "collector-ha",
			"entity_id":    "light.last_success",
			"domain":       "light",
			"state":        "off",
		},
	}); err != nil {
		t.Fatalf("seed Home Assistant child: %v", err)
	}

	secretsManager, err := secrets.NewManagerFromEncodedKey("MDEyMzQ1Njc4OUFCQ0RFRjAxMjM0NTY3ODlBQkNERUY=")
	if err != nil {
		t.Fatalf("create secrets manager: %v", err)
	}
	credentialStore := persistence.NewMemoryCredentialStore()
	ciphertext, err := secretsManager.EncryptString("ha-test-token", "credential-ha")
	if err != nil {
		t.Fatalf("encrypt credential: %v", err)
	}
	if _, err := credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               "credential-ha",
		Name:             "Home Assistant QA",
		Kind:             credentials.KindHomeAssistantToken,
		Status:           "active",
		SecretCiphertext: ciphertext,
	}); err != nil {
		t.Fatalf("create credential profile: %v", err)
	}

	collector := hubcollector.Collector{
		ID:              "collector-ha",
		AssetID:         "ha-root",
		CollectorType:   hubcollector.CollectorTypeHomeAssistant,
		Enabled:         true,
		IntervalSeconds: hubcollector.DefaultIntervalSeconds,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "credential-ha",
			"display_name":  "Disposable HA",
		},
	}
	statusStore := &homeAssistantCollectorStatusStore{collector: collector}
	deps := &Deps{
		AssetStore:        assetStore,
		HubCollectorStore: statusStore,
		CredentialStore:   credentialStore,
		SecretsManager:    secretsManager,
		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			stored, err := assetStore.UpsertAssetHeartbeat(req)
			return &stored, err
		},
		PersistCanonicalConnectorSnapshot: func(string, string, string, string, connectorsdk.Connector, []connectorsdk.Asset) {},
	}

	deps.executeHomeAssistantCollector(context.Background(), collector)

	offlineRoot, ok, err := assetStore.GetAsset("ha-root")
	if err != nil || !ok {
		t.Fatalf("load root after outage: ok=%v err=%v", ok, err)
	}
	if offlineRoot.Status != "offline" {
		t.Fatalf("root status after outage = %q, want offline", offlineRoot.Status)
	}
	if offlineRoot.Name != "Disposable HA" {
		t.Fatalf("outage replaced root name: %q", offlineRoot.Name)
	}
	if offlineRoot.Metadata["discovered"] != "7" || offlineRoot.Metadata["stale_marker"] != "preserve-me" {
		t.Fatalf("outage replaced last successful root metadata: %#v", offlineRoot.Metadata)
	}
	if _, ok, err := assetStore.GetAsset("ha-entity-light-last-success"); err != nil || !ok {
		t.Fatalf("outage deleted last successful child inventory: ok=%v err=%v", ok, err)
	}
	if statusStore.collector.LastStatus != "error" || !strings.Contains(statusStore.collector.LastError, "home assistant discovery failed") {
		t.Fatalf("collector outage status = %q error=%q", statusStore.collector.LastStatus, statusStore.collector.LastError)
	}

	available = true
	deps.executeHomeAssistantCollector(context.Background(), collector)

	recoveredRoot, ok, err := assetStore.GetAsset("ha-root")
	if err != nil || !ok {
		t.Fatalf("load root after recovery: ok=%v err=%v", ok, err)
	}
	if recoveredRoot.Status != "online" {
		t.Fatalf("root status after successful recovery = %q, want online", recoveredRoot.Status)
	}
	if recoveredRoot.Metadata["discovered"] != "1" || recoveredRoot.Metadata["ha_version"] != "2026.7.0" {
		t.Fatalf("recovery did not publish fresh root metadata: %#v", recoveredRoot.Metadata)
	}
	if _, ok, err := assetStore.GetAsset("ha-entity-light-last-success"); err != nil || !ok {
		t.Fatalf("successful refresh deleted stale child inventory: ok=%v err=%v", ok, err)
	}
	if statusStore.collector.LastStatus != "ok" || statusStore.collector.LastError != "" {
		t.Fatalf("collector recovery status = %q error=%q", statusStore.collector.LastStatus, statusStore.collector.LastError)
	}
}
