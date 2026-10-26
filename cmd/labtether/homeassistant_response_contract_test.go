package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
)

// The CLI decodes every v2 success through data. A raw legacy response can
// return HTTP 200 while silently hiding all Home Assistant entities from it.
func TestHomeAssistantV2ResponseContract(t *testing.T) {
	s := newTestAPIServer(t)
	for _, domain := range []string{"light", "automation", "scene"} {
		_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "ha-" + domain, Type: "ha-entity", Name: domain,
			Source: "homeassistant", Status: "online",
			Metadata: map[string]string{"entity_id": domain + ".qa", "domain": domain},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		name    string
		path    string
		handler http.HandlerFunc
		count   int
		id      string
	}{
		{"entities", "/api/v2/homeassistant/entities", s.handleV2HAEntities, 3, ""},
		{"entity", "/api/v2/homeassistant/entities/ha-light", s.handleV2HAEntityActions, 0, "ha-light"},
		{"automations", "/api/v2/homeassistant/automations", s.handleV2HAAutomations, 1, "ha-automation"},
		{"scenes", "/api/v2/homeassistant/scenes", s.handleV2HAScenes, 1, "ha-scene"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			ctx := contextWithPrincipal(req.Context(), "apikey:ha-read", "operator")
			req = req.WithContext(contextWithScopes(ctx, []string{"homeassistant:read"}))
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var response struct {
				RequestID string `json:"request_id"`
				Data      *struct {
					Entities []assets.Asset `json:"entities"`
					Entity   assets.Asset   `json:"entity"`
					Count    int            `json:"count"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.RequestID == "" || response.Data == nil {
				t.Fatalf("missing v2 envelope: %s", rec.Body.String())
			}
			if tc.count == 0 {
				if response.Data.Entity.ID != tc.id {
					t.Fatalf("entity = %q, want %q", response.Data.Entity.ID, tc.id)
				}
			} else if response.Data.Count != tc.count || len(response.Data.Entities) != tc.count {
				t.Fatalf("collection data differs: %s", rec.Body.String())
			} else if tc.id != "" && response.Data.Entities[0].ID != tc.id {
				t.Fatalf("collection contains wrong entity: %s", rec.Body.String())
			}
		})
	}

	for _, tc := range []struct {
		name, method, path string
		handler            http.HandlerFunc
		status             int
	}{
		{"missing", http.MethodGet, "/api/v2/homeassistant/entities/missing", s.handleV2HAEntityActions, http.StatusNotFound},
		{"scope denied", http.MethodPost, "/api/v2/homeassistant/entities", s.handleV2HAEntities, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			ctx := contextWithPrincipal(req.Context(), "apikey:ha-read", "operator")
			req = req.WithContext(contextWithScopes(ctx, []string{"homeassistant:read"}))
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			var response struct {
				RequestID string          `json:"request_id"`
				Error     string          `json:"error"`
				Data      json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tc.status || response.RequestID == "" || response.Error == "" || response.Data != nil {
				t.Fatalf("invalid v2 error: status %d, body %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHomeAssistantV2ResponseContractActionFailure(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/services/light/turn_on" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer qa-ha-token" {
			t.Error("upstream auth header differs")
		}
		var body struct {
			EntityID string `json:"entity_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.EntityID != "light.qa" {
			t.Errorf("upstream entity=%q, decode error=%v", body.EntityID, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"private details https://internal.example/ qa-ha-token"}`))
	}))
	t.Cleanup(upstream.Close)

	s := newTestAPIServer(t)
	createHomeAssistantCredentialProfile(t, s, "cred-ha", "qa-ha-token", upstream.URL)
	s.hubCollectorStore = &stubHubCollectorStore{collectors: []hubcollector.Collector{{
		ID: "collector-ha", CollectorType: hubcollector.CollectorTypeHomeAssistant, Enabled: true,
		Config: map[string]any{"base_url": upstream.URL, "credential_id": "cred-ha", "timeout": "5s"},
	}}}
	if _, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "ha-light", Type: "ha-entity", Name: "QA light", Source: "homeassistant", Status: "online",
		Metadata: map[string]string{"entity_id": "light.qa", "domain": "light", "collector_id": "collector-ha"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/homeassistant/entities/ha-light", strings.NewReader(`{"action":"turn_on"}`))
	ctx := contextWithPrincipal(req.Context(), "apikey:ha-admin", "admin")
	req = req.WithContext(contextWithScopes(ctx, []string{"homeassistant:write"}))
	rec := httptest.NewRecorder()
	s.handleV2HAEntityActions(rec, req)
	var response struct {
		RequestID string          `json:"request_id"`
		Error     string          `json:"error"`
		Message   string          `json:"message"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls=%d, want 1; response=%s", calls.Load(), rec.Body.String())
	}
	if rec.Code != http.StatusBadRequest || response.RequestID == "" || response.Data != nil ||
		response.Error != "homeassistant_action_failed" || response.Message != "home assistant api returned 400" {
		t.Fatalf("action failure lost its v2 error or cause: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "internal.example") || strings.Contains(rec.Body.String(), "qa-ha-token") {
		t.Fatalf("action failure exposed upstream details: %s", rec.Body.String())
	}
}
