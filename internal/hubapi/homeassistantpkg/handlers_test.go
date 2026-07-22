package homeassistantpkg

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

type haCollectorStore struct {
	collectors []hubcollector.Collector
	listErr    error
}

func (s *haCollectorStore) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	collector := hubcollector.Collector{ID: "collector-created", AssetID: req.AssetID, CollectorType: req.CollectorType, Config: req.Config, Enabled: true}
	s.collectors = append(s.collectors, collector)
	return collector, nil
}
func (s *haCollectorStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	for _, collector := range s.collectors {
		if collector.ID == id {
			return collector, true, nil
		}
	}
	return hubcollector.Collector{}, false, nil
}
func (s *haCollectorStore) ListHubCollectors(_ int, enabledOnly bool) ([]hubcollector.Collector, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	result := make([]hubcollector.Collector, 0, len(s.collectors))
	for _, collector := range s.collectors {
		if !enabledOnly || collector.Enabled {
			result = append(result, collector)
		}
	}
	return result, nil
}
func (s *haCollectorStore) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	collector, ok, _ := s.GetHubCollector(id)
	if !ok {
		return hubcollector.Collector{}, hubcollector.ErrCollectorNotFound
	}
	if req.Config != nil {
		collector.Config = *req.Config
	}
	return collector, nil
}
func (s *haCollectorStore) DeleteHubCollector(string) error { return nil }
func (s *haCollectorStore) UpdateHubCollectorStatus(string, string, string, time.Time) error {
	return nil
}

type haRuntimeFixture struct {
	deps         *Deps
	lightAssetID string
	requestsMu   sync.Mutex
	requests     []string
}

func newHARuntimeFixture(t *testing.T) *haRuntimeFixture {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	fixture := &haRuntimeFixture{}
	haServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer qa-ha-token" {
			t.Errorf("Authorization header mismatch")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fixture.requestsMu.Lock()
		fixture.requests = append(fixture.requests, r.Method+" "+r.URL.Path)
		fixture.requestsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/services/homeassistant/toggle", "/api/services/light/turn_on":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode service body: %v", err)
			}
			if body["entity_id"] != "light.qa_lamp" {
				t.Errorf("entity_id = %#v", body["entity_id"])
			}
			_, _ = w.Write([]byte(`[{"entity_id":"light.qa_lamp","state":"on"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		}
	}))
	t.Cleanup(haServer.Close)

	manager, err := secrets.NewManagerFromEncodedKey(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := manager.EncryptString("qa-ha-token", "cred-ha-qa")
	if err != nil {
		t.Fatal(err)
	}
	credentialStore := persistence.NewMemoryCredentialStore()
	if _, err := credentialStore.CreateCredentialProfile(credentials.Profile{
		ID: "cred-ha-qa", Name: "HA QA", Kind: credentials.KindHomeAssistantToken, SecretCiphertext: ciphertext,
	}); err != nil {
		t.Fatal(err)
	}

	collectorID := "collector-ha-qa"
	collectorStore := &haCollectorStore{collectors: []hubcollector.Collector{{
		ID: collectorID, AssetID: "ha-cluster", CollectorType: hubcollector.CollectorTypeHomeAssistant, Enabled: true,
		Config: map[string]any{"base_url": haServer.URL, "credential_id": "cred-ha-qa", "timeout": "5s"},
	}}}
	assetStore := persistence.NewMemoryAssetStore()
	add := func(nativeID, entityID, domain, state string) string {
		id := assetid.ScopeCollectorAssetID(nativeID, collectorID)
		_, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id, Type: "ha-entity", Name: entityID, Source: "homeassistant", Status: "online",
			Metadata: map[string]string{"entity_id": entityID, "domain": domain, "state": state, "collector_id": collectorID},
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	fixture.lightAssetID = add("ha-entity-light-qa-lamp", "light.qa_lamp", "light", "off")
	add("ha-entity-automation-qa", "automation.qa", "automation", "on")
	add("ha-entity-scene-qa", "scene.qa", "scene", "scening")
	_, _ = assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "other", Type: "agent", Name: "other", Source: "agent", Status: "online"})

	fixture.deps = &Deps{
		AssetStore: assetStore, HubCollectorStore: collectorStore, CredentialStore: credentialStore, SecretsManager: manager,
		RequireAdminAuth: func(http.ResponseWriter, *http.Request) bool { return true },
	}
	return fixture
}

func scopedRequest(method, target, body string, scopes []string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := apiv2.ContextWithScopes(req.Context(), scopes)
	return req.WithContext(ctx)
}

func TestHomeAssistantEntityCollectionsAreFunctionalAndFiltered(t *testing.T) {
	fixture := newHARuntimeFixture(t)

	for _, test := range []struct {
		path      string
		wantCount int
	}{
		{path: "/api/v2/homeassistant/entities", wantCount: 3},
		{path: "/api/v2/homeassistant/entities?domain=light", wantCount: 1},
		{path: "/api/v2/homeassistant/automations", wantCount: 1},
		{path: "/api/v2/homeassistant/scenes", wantCount: 1},
	} {
		req := scopedRequest(http.MethodGet, test.path, "", []string{"homeassistant:read"})
		rec := httptest.NewRecorder()
		switch {
		case strings.Contains(test.path, "/automations"):
			fixture.deps.HandleV2HAAutomations(rec, req)
		case strings.Contains(test.path, "/scenes"):
			fixture.deps.HandleV2HAScenes(rec, req)
		default:
			fixture.deps.HandleV2HAEntities(rec, req)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", test.path, rec.Code, rec.Body.String())
		}
		var payload struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || payload.Count != test.wantCount {
			t.Fatalf("GET %s payload=%s err=%v want count=%d", test.path, rec.Body.String(), err, test.wantCount)
		}
	}
}

func TestHomeAssistantEntityListHonorsAssetAllowlist(t *testing.T) {
	fixture := newHARuntimeFixture(t)
	req := scopedRequest(http.MethodGet, "/api/v2/homeassistant/entities", "", []string{"homeassistant:read"})
	req = req.WithContext(apiv2.ContextWithAllowedAssets(req.Context(), []string{fixture.lightAssetID}))
	rec := httptest.NewRecorder()
	fixture.deps.HandleV2HAEntities(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), fixture.lightAssetID) || strings.Contains(rec.Body.String(), "automation.qa") {
		t.Fatalf("allowlisted list status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHomeAssistantEntityActionToggleAndService(t *testing.T) {
	fixture := newHARuntimeFixture(t)
	path := "/api/v2/homeassistant/entities/" + fixture.lightAssetID

	for _, body := range []string{`{"action":"toggle"}`, `{"action":"turn_on"}`} {
		req := scopedRequest(http.MethodPost, path, body, []string{"homeassistant:write"})
		rec := httptest.NewRecorder()
		fixture.deps.HandleV2HAEntityActions(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"succeeded"`) {
			t.Fatalf("action body=%s status=%d response=%s", body, rec.Code, rec.Body.String())
		}
	}

	fixture.requestsMu.Lock()
	requests := append([]string(nil), fixture.requests...)
	fixture.requestsMu.Unlock()
	if len(requests) != 2 || requests[0] != "POST /api/services/homeassistant/toggle" || requests[1] != "POST /api/services/light/turn_on" {
		t.Fatalf("upstream requests = %v", requests)
	}
}

func TestHomeAssistantEntityActionScopeAndDomainFailClosed(t *testing.T) {
	fixture := newHARuntimeFixture(t)
	path := "/api/v2/homeassistant/entities/" + fixture.lightAssetID

	readOnly := scopedRequest(http.MethodPost, path, `{"action":"toggle"}`, []string{"homeassistant:read"})
	readOnlyRec := httptest.NewRecorder()
	fixture.deps.HandleV2HAEntityActions(readOnlyRec, readOnly)
	if readOnlyRec.Code != http.StatusForbidden {
		t.Fatalf("read-only action status=%d", readOnlyRec.Code)
	}

	crossDomain := scopedRequest(http.MethodPost, path, `{"action":"service.call","service":"switch.turn_on"}`, []string{"homeassistant:write"})
	crossDomainRec := httptest.NewRecorder()
	fixture.deps.HandleV2HAEntityActions(crossDomainRec, crossDomain)
	if crossDomainRec.Code != http.StatusBadRequest || !strings.Contains(crossDomainRec.Body.String(), "domain") {
		t.Fatalf("cross-domain action status=%d body=%s", crossDomainRec.Code, crossDomainRec.Body.String())
	}
}

func TestHomeAssistantEntityMutationsRequireAdmin(t *testing.T) {
	fixture := newHARuntimeFixture(t)
	fixture.deps.RequireAdminAuth = func(w http.ResponseWriter, _ *http.Request) bool {
		http.Error(w, "admin required", http.StatusForbidden)
		return false
	}
	path := "/api/v2/homeassistant/entities/" + fixture.lightAssetID
	req := scopedRequest(http.MethodPost, path, `{"action":"toggle"}`, []string{"homeassistant:write"})
	rec := httptest.NewRecorder()
	fixture.deps.HandleV2HAEntityActions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin mutation status=%d body=%s", rec.Code, rec.Body.String())
	}
	fixture.requestsMu.Lock()
	defer fixture.requestsMu.Unlock()
	if len(fixture.requests) != 0 {
		t.Fatalf("non-admin mutation reached upstream: %v", fixture.requests)
	}
}

func TestHomeAssistantRuntimeResolutionErrorsDoNotLeakInternals(t *testing.T) {
	fixture := newHARuntimeFixture(t)
	fixture.deps.HubCollectorStore = &haCollectorStore{listErr: errors.New("postgres://secret@internal-db:5432/labtether")}
	path := "/api/v2/homeassistant/entities/" + fixture.lightAssetID
	req := scopedRequest(http.MethodPost, path, `{"action":"toggle"}`, []string{"homeassistant:write"})
	rec := httptest.NewRecorder()
	fixture.deps.HandleV2HAEntityActions(rec, req)
	if rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "internal-db") {
		t.Fatalf("runtime resolution leaked internals: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHomeAssistantEntityGetAndUnsupportedMethods(t *testing.T) {
	fixture := newHARuntimeFixture(t)
	path := "/api/v2/homeassistant/entities/" + fixture.lightAssetID

	req := scopedRequest(http.MethodGet, path, "", []string{"homeassistant:read"})
	rec := httptest.NewRecorder()
	fixture.deps.HandleV2HAEntityActions(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "light.qa_lamp") {
		t.Fatalf("entity get status=%d body=%s", rec.Code, rec.Body.String())
	}

	deleteReq := scopedRequest(http.MethodDelete, path, "", []string{"homeassistant:write"})
	deleteRec := httptest.NewRecorder()
	fixture.deps.HandleV2HAEntityActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("entity DELETE status=%d", deleteRec.Code)
	}
}

func TestResolveEntityActionRejectsCrossDomainAndSupportsFriendlyActions(t *testing.T) {
	asset := assets.Asset{Metadata: map[string]string{"domain": "light"}}
	for _, action := range []string{"toggle", "turn_on", "light.turn_off"} {
		resolved, _, err := resolveEntityAction(asset, entityActionRequest{Action: action})
		if err != nil || (resolved != "entity.toggle" && resolved != "service.call") {
			t.Fatalf("action=%q resolved=%q err=%v", action, resolved, err)
		}
	}
	if _, _, err := resolveEntityAction(asset, entityActionRequest{Action: "switch.turn_on"}); err == nil {
		t.Fatal("cross-domain service unexpectedly accepted")
	}
	if _, _, err := resolveEntityAction(asset, entityActionRequest{Action: "homeassistant.restart"}); err == nil {
		t.Fatal("global Home Assistant restart unexpectedly accepted as an entity action")
	}
	if _, _, err := resolveEntityAction(asset, entityActionRequest{Action: "homeassistant.toggle"}); err != nil {
		t.Fatalf("safe universal entity toggle rejected: %v", err)
	}
}

var _ persistence.HubCollectorStore = (*haCollectorStore)(nil)
