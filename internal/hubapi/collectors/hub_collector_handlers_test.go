package collectors

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

type hubCollectorHandlerStore struct {
	collector    hubcollector.Collector
	createCalls  int
	updateCalls  int
	statusCalls  int
	deleteCalled bool
	createConfig map[string]any
}

func (s *hubCollectorHandlerStore) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	s.createCalls++
	interval, err := hubcollector.CreateIntervalSeconds(req.IntervalSeconds)
	if err != nil {
		return hubcollector.Collector{}, err
	}
	config := req.Config
	if s.createConfig != nil {
		config = s.createConfig
	}
	s.collector = hubcollector.Collector{
		ID:              "collector-1",
		AssetID:         req.AssetID,
		CollectorType:   req.CollectorType,
		Config:          config,
		IntervalSeconds: interval,
		Enabled:         true,
	}
	return s.collector, nil
}

func (s *hubCollectorHandlerStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	if s.collector.ID == id {
		return s.collector, true, nil
	}
	return hubcollector.Collector{}, false, nil
}

func (s *hubCollectorHandlerStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	if s.collector.ID == "" {
		return nil, nil
	}
	return []hubcollector.Collector{s.collector}, nil
}

func (s *hubCollectorHandlerStore) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	s.updateCalls++
	if s.collector.ID != id {
		return hubcollector.Collector{}, hubcollector.ErrCollectorNotFound
	}
	if req.IntervalSeconds != nil {
		if err := hubcollector.ValidateIntervalSeconds(*req.IntervalSeconds); err != nil {
			return hubcollector.Collector{}, err
		}
		s.collector.IntervalSeconds = *req.IntervalSeconds
	}
	if req.Config != nil {
		s.collector.Config = *req.Config
	}
	if req.Enabled != nil {
		s.collector.Enabled = *req.Enabled
	}
	return s.collector, nil
}

func (s *hubCollectorHandlerStore) DeleteHubCollector(id string) error {
	s.deleteCalled = true
	return nil
}

func (s *hubCollectorHandlerStore) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	s.statusCalls++
	return nil
}

func newHubCollectorHandlerDeps(store *hubCollectorHandlerStore) *Deps {
	return &Deps{
		HubCollectorStore: store,
		EnforceRateLimit: func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool {
			return true
		},
	}
}

func TestHandleHubCollectorsRejectsOutOfRangeIntervalOnCreate(t *testing.T) {
	deps := newHubCollectorHandlerDeps(&hubCollectorHandlerStore{})
	payload := []byte(`{"asset_id":"asset-1","collector_type":"ssh","interval_seconds":2147483648}`)
	req := httptest.NewRequest(http.MethodPost, "/hub-collectors", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.HandleHubCollectors(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleHubCollectorActionsRejectsOutOfRangeIntervalOnUpdate(t *testing.T) {
	store := &hubCollectorHandlerStore{
		collector: hubcollector.Collector{
			ID:              "collector-1",
			AssetID:         "asset-1",
			CollectorType:   hubcollector.CollectorTypeSSH,
			IntervalSeconds: hubcollector.DefaultIntervalSeconds,
		},
	}
	deps := newHubCollectorHandlerDeps(store)
	payload := []byte(`{"interval_seconds":2147483648}`)
	req := httptest.NewRequest(http.MethodPatch, "/hub-collectors/collector-1", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.HandleHubCollectorActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRestrictedHubCollectorObjectAccessEnforcesCollectorAsset(t *testing.T) {
	store := &hubCollectorHandlerStore{
		collector: hubcollector.Collector{
			ID:              "collector-1",
			AssetID:         "asset-secret",
			CollectorType:   hubcollector.CollectorTypeSSH,
			IntervalSeconds: hubcollector.DefaultIntervalSeconds,
		},
	}
	d := newHubCollectorHandlerDeps(store)
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-allowed"})
	req := httptest.NewRequest(http.MethodDelete, "/hub-collectors/collector-1", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleHubCollectorActions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if store.deleteCalled {
		t.Fatal("forbidden delete reached store")
	}
	if _, ok, err := store.GetHubCollector("collector-1"); err != nil || !ok {
		t.Fatalf("forbidden delete reached store: ok=%v err=%v", ok, err)
	}
}

func newCollectorCredentialStore(t *testing.T) *persistence.MemoryCredentialStore {
	t.Helper()
	store := persistence.NewMemoryCredentialStore()
	if _, err := store.CreateCredentialProfile(credentials.Profile{
		ID:   "credential-profile-1",
		Name: "collector test profile",
		Kind: credentials.KindSSHPassword,
	}); err != nil {
		t.Fatalf("create credential profile: %v", err)
	}
	return store
}

func TestHandleHubCollectorsRejectsInlineSecretConfigOnCreate(t *testing.T) {
	const secretValue = "synthetic-inline-sensitive-value"
	tests := []struct {
		name   string
		config map[string]any
	}{
		{name: "password", config: map[string]any{"password": secretValue}},
		{name: "empty password field", config: map[string]any{"password": ""}},
		{name: "dashed private key", config: map[string]any{"private-key": secretValue}},
		{name: "camel case client secret", config: map[string]any{"clientSecret": secretValue}},
		{name: "mixed case api key", config: map[string]any{"API_Key": secretValue}},
		{name: "access token", config: map[string]any{"accessToken": secretValue}},
		{name: "noncanonical token id", config: map[string]any{"token-id": secretValue}},
		{name: "noncanonical api key header", config: map[string]any{"apiKeyHeader": secretValue}},
		{name: "short password variant", config: map[string]any{"login_pwd": secretValue}},
		{name: "nested authorization header", config: map[string]any{
			"headers": map[string]any{"Authorization": "Bearer " + secretValue},
		}},
		{name: "secret nested in array", config: map[string]any{
			"steps": []any{map[string]any{"x-api-key": secretValue}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &hubCollectorHandlerStore{}
			deps := newHubCollectorHandlerDeps(store)
			payload, err := json.Marshal(map[string]any{
				"asset_id":         "asset-1",
				"collector_type":   "ssh",
				"interval_seconds": 60,
				"config":           tt.config,
			})
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/hub-collectors", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			deps.HandleHubCollectors(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			if store.createCalls != 0 {
				t.Fatalf("secret-bearing request reached persistence: createCalls=%d", store.createCalls)
			}
			if strings.Contains(rec.Body.String(), secretValue) {
				t.Fatal("error response reflected inline secret material")
			}
		})
	}
}

func TestHandleHubCollectorActionsRejectsInlineSecretConfigOnUpdate(t *testing.T) {
	const secretValue = "synthetic-update-sensitive-value"
	store := &hubCollectorHandlerStore{collector: hubcollector.Collector{
		ID:              "collector-1",
		AssetID:         "asset-1",
		CollectorType:   hubcollector.CollectorTypeSSH,
		IntervalSeconds: hubcollector.DefaultIntervalSeconds,
		Config:          map[string]any{"base_url": "https://example.invalid"},
	}}
	deps := newHubCollectorHandlerDeps(store)
	payload := []byte(`{"config":{"nested":{"token_secret":"synthetic-update-sensitive-value"}}}`)
	req := httptest.NewRequest(http.MethodPatch, "/hub-collectors/collector-1", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.HandleHubCollectorActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if store.updateCalls != 0 {
		t.Fatalf("secret-bearing update reached persistence: updateCalls=%d", store.updateCalls)
	}
	if strings.Contains(rec.Body.String(), secretValue) {
		t.Fatal("error response reflected inline secret material")
	}
}

func TestHubCollectorReadResponsesRedactLegacySecretsWithoutMutatingStore(t *testing.T) {
	const (
		passwordValue = "synthetic-legacy-password"
		tokenValue    = "synthetic-legacy-token"
	)
	nested := map[string]any{
		"clientSecret": tokenValue,
		"safe":         "preserved",
	}
	store := &hubCollectorHandlerStore{collector: hubcollector.Collector{
		ID:              "collector-1",
		AssetID:         "asset-1",
		CollectorType:   hubcollector.CollectorTypeAPI,
		IntervalSeconds: hubcollector.DefaultIntervalSeconds,
		Config: map[string]any{
			"base_url":       "https://example.invalid",
			"credential_id":  "credential-profile-1",
			"token_id":       "operator@realm!collector",
			"api_key_header": "X-API-Key",
			"password":       passwordValue,
			"nested":         nested,
			"items": []any{
				map[string]any{"authorization": "Bearer " + tokenValue},
			},
		},
	}}
	deps := newHubCollectorHandlerDeps(store)

	tests := []struct {
		name   string
		path   string
		handle func(http.ResponseWriter, *http.Request)
	}{
		{name: "single", path: "/hub-collectors/collector-1", handle: deps.HandleHubCollectorActions},
		{name: "list", path: "/hub-collectors", handle: deps.HandleHubCollectors},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			tt.handle(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if strings.Contains(body, passwordValue) || strings.Contains(body, tokenValue) {
				t.Fatal("collector response exposed persisted secret material")
			}
			if !strings.Contains(body, RedactedConnectorSecret) {
				t.Fatalf("collector response did not contain redaction marker: %s", body)
			}
			for _, safeValue := range []string{
				"https://example.invalid",
				"credential-profile-1",
				"operator@realm!collector",
				"X-API-Key",
				"preserved",
			} {
				if !strings.Contains(body, safeValue) {
					t.Fatalf("collector response lost safe config value %q", safeValue)
				}
			}
		})
	}

	if got := store.collector.Config["password"]; got != passwordValue {
		t.Fatalf("redaction mutated stored password: got %#v", got)
	}
	if got := nested["clientSecret"]; got != tokenValue {
		t.Fatalf("redaction mutated nested stored secret: got %#v", got)
	}
}

func TestHubCollectorWriteResponsesRedactLegacySecrets(t *testing.T) {
	const secretValue = "synthetic-store-sensitive-value"

	t.Run("create response", func(t *testing.T) {
		store := &hubCollectorHandlerStore{createConfig: map[string]any{
			"base_url": "https://example.invalid",
			"password": secretValue,
		}}
		deps := newHubCollectorHandlerDeps(store)
		req := httptest.NewRequest(http.MethodPost, "/hub-collectors", strings.NewReader(
			`{"asset_id":"asset-1","collector_type":"ssh","config":{"base_url":"https://example.invalid"}}`,
		))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		deps.HandleHubCollectors(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), secretValue) || !strings.Contains(rec.Body.String(), RedactedConnectorSecret) {
			t.Fatalf("create response was not redacted: %s", rec.Body.String())
		}
	})

	t.Run("update response", func(t *testing.T) {
		store := &hubCollectorHandlerStore{collector: hubcollector.Collector{
			ID:              "collector-1",
			AssetID:         "asset-1",
			CollectorType:   hubcollector.CollectorTypeSSH,
			IntervalSeconds: hubcollector.DefaultIntervalSeconds,
			Config:          map[string]any{"password": secretValue},
		}}
		deps := newHubCollectorHandlerDeps(store)
		req := httptest.NewRequest(http.MethodPatch, "/hub-collectors/collector-1", strings.NewReader(`{"interval_seconds":61}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		deps.HandleHubCollectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), secretValue) || !strings.Contains(rec.Body.String(), RedactedConnectorSecret) {
			t.Fatalf("update response was not redacted: %s", rec.Body.String())
		}
	})
}

func TestHubCollectorCreateCredentialBindingRequiresScopeAndExistingProfile(t *testing.T) {
	credentialStore := newCollectorCredentialStore(t)
	tests := []struct {
		name         string
		credentialID string
		scopes       []string
		wantStatus   int
		wantCreate   bool
	}{
		{
			name:         "missing credentials use scope",
			credentialID: "credential-profile-1",
			scopes:       []string{"collectors:write"},
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "missing credential profile",
			credentialID: "credential-profile-missing",
			scopes:       []string{"collectors:write", "credentials:use"},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "authorized existing profile",
			credentialID: "credential-profile-1",
			scopes:       []string{"collectors:write", "credentials:use"},
			wantStatus:   http.StatusCreated,
			wantCreate:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &hubCollectorHandlerStore{}
			deps := newHubCollectorHandlerDeps(store)
			deps.CredentialStore = credentialStore
			payload, err := json.Marshal(map[string]any{
				"asset_id":       "asset-1",
				"collector_type": "api",
				"config": map[string]any{
					"credential_id":  tt.credentialID,
					"token_id":       "operator@realm!collector",
					"api_key_header": "X-API-Key",
				},
			})
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			ctx := apiv2.ContextWithScopes(context.Background(), tt.scopes)
			req := httptest.NewRequest(http.MethodPost, "/hub-collectors", bytes.NewReader(payload)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			deps.HandleHubCollectors(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d body=%s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if got := store.createCalls > 0; got != tt.wantCreate {
				t.Fatalf("create persistence call=%v, want %v", got, tt.wantCreate)
			}
		})
	}
}

func TestHubCollectorCreateRejectsNonStringCredentialID(t *testing.T) {
	store := &hubCollectorHandlerStore{}
	deps := newHubCollectorHandlerDeps(store)
	req := httptest.NewRequest(http.MethodPost, "/hub-collectors", strings.NewReader(
		`{"asset_id":"asset-1","collector_type":"ssh","config":{"credential_id":42}}`,
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.HandleHubCollectors(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if store.createCalls != 0 {
		t.Fatalf("invalid credential binding reached persistence: createCalls=%d", store.createCalls)
	}
}

func TestHubCollectorUpdateCredentialBindingRequiresScopeAndExistingProfile(t *testing.T) {
	credentialStore := newCollectorCredentialStore(t)
	tests := []struct {
		name         string
		credentialID string
		scopes       []string
		wantStatus   int
		wantUpdate   bool
	}{
		{
			name:         "missing credentials use scope",
			credentialID: "credential-profile-1",
			scopes:       []string{"collectors:write"},
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "missing credential profile",
			credentialID: "credential-profile-missing",
			scopes:       []string{"collectors:write", "credentials:use"},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "authorized existing profile",
			credentialID: "credential-profile-1",
			scopes:       []string{"collectors:write", "credentials:use"},
			wantStatus:   http.StatusOK,
			wantUpdate:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &hubCollectorHandlerStore{collector: hubcollector.Collector{
				ID:              "collector-1",
				AssetID:         "asset-1",
				CollectorType:   hubcollector.CollectorTypeSSH,
				IntervalSeconds: hubcollector.DefaultIntervalSeconds,
				Config:          map[string]any{"host": "example.invalid"},
			}}
			deps := newHubCollectorHandlerDeps(store)
			deps.CredentialStore = credentialStore
			payload, err := json.Marshal(map[string]any{
				"config": map[string]any{
					"host":          "example.invalid",
					"credential_id": tt.credentialID,
				},
			})
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			ctx := apiv2.ContextWithScopes(context.Background(), tt.scopes)
			req := httptest.NewRequest(http.MethodPatch, "/hub-collectors/collector-1", bytes.NewReader(payload)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			deps.HandleHubCollectorActions(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d body=%s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if got := store.updateCalls > 0; got != tt.wantUpdate {
				t.Fatalf("update persistence call=%v, want %v", got, tt.wantUpdate)
			}
		})
	}
}

func TestHubCollectorUpdateThatInvokesExistingBindingRequiresCredentialsUse(t *testing.T) {
	credentialStore := newCollectorCredentialStore(t)
	tests := []struct {
		name            string
		existingEnabled bool
		credentialID    string
		payload         string
		scopes          []string
		wantStatus      int
		wantUpdate      bool
	}{
		{
			name:         "enable requires scope",
			credentialID: "credential-profile-1",
			payload:      `{"enabled":true}`,
			scopes:       []string{"collectors:write"},
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "enable validates retained profile",
			credentialID: "credential-profile-missing",
			payload:      `{"enabled":true}`,
			scopes:       []string{"collectors:write", "credentials:use"},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:            "reschedule enabled collector requires scope",
			existingEnabled: true,
			credentialID:    "credential-profile-1",
			payload:         `{"interval_seconds":1}`,
			scopes:          []string{"collectors:write"},
			wantStatus:      http.StatusForbidden,
		},
		{
			name:            "disable remains an unrestricted kill switch",
			existingEnabled: true,
			credentialID:    "credential-profile-missing",
			payload:         `{"enabled":false}`,
			scopes:          []string{"collectors:write"},
			wantStatus:      http.StatusOK,
			wantUpdate:      true,
		},
		{
			name:         "reschedule disabled collector does not invoke credential",
			credentialID: "credential-profile-1",
			payload:      `{"interval_seconds":120}`,
			scopes:       []string{"collectors:write"},
			wantStatus:   http.StatusOK,
			wantUpdate:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &hubCollectorHandlerStore{collector: hubcollector.Collector{
				ID:              "collector-1",
				AssetID:         "asset-1",
				CollectorType:   hubcollector.CollectorTypeSSH,
				Enabled:         tt.existingEnabled,
				IntervalSeconds: hubcollector.DefaultIntervalSeconds,
				Config:          map[string]any{"credential_id": tt.credentialID},
			}}
			deps := newHubCollectorHandlerDeps(store)
			deps.CredentialStore = credentialStore
			ctx := apiv2.ContextWithScopes(context.Background(), tt.scopes)
			req := httptest.NewRequest(http.MethodPatch, "/hub-collectors/collector-1", strings.NewReader(tt.payload)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			deps.HandleHubCollectorActions(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d body=%s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if got := store.updateCalls > 0; got != tt.wantUpdate {
				t.Fatalf("update persistence call=%v, want %v", got, tt.wantUpdate)
			}
		})
	}
}

func TestHubCollectorRunCredentialBindingRequiresScopeAndExistingProfile(t *testing.T) {
	credentialStore := newCollectorCredentialStore(t)
	tests := []struct {
		name         string
		credentialID string
		scopes       []string
		wantStatus   int
	}{
		{
			name:         "missing credentials use scope",
			credentialID: "credential-profile-1",
			scopes:       []string{"collectors:write"},
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "missing credential profile",
			credentialID: "credential-profile-missing",
			scopes:       []string{"collectors:write", "credentials:use"},
			wantStatus:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &hubCollectorHandlerStore{collector: hubcollector.Collector{
				ID:              "collector-1",
				AssetID:         "asset-1",
				CollectorType:   hubcollector.CollectorTypeSSH,
				IntervalSeconds: hubcollector.DefaultIntervalSeconds,
				Config:          map[string]any{"credential_id": tt.credentialID},
			}}
			deps := newHubCollectorHandlerDeps(store)
			deps.CredentialStore = credentialStore
			ctx := apiv2.ContextWithScopes(context.Background(), tt.scopes)
			req := httptest.NewRequest(http.MethodPost, "/hub-collectors/collector-1/run", nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			deps.HandleHubCollectorActions(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d body=%s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if store.statusCalls != 0 {
				t.Fatalf("unauthorized collector run started: statusCalls=%d", store.statusCalls)
			}
		})
	}
}

func TestConnectorCredentialInvocationRequiresCredentialsUseEvenWithInlineSecret(t *testing.T) {
	const secretValue = "synthetic-connector-sensitive-value"
	credentialStore := newCollectorCredentialStore(t)
	tests := []struct {
		name    string
		payload string
		handle  func(*Deps, http.ResponseWriter, *http.Request)
	}{
		{
			name:    "proxmox",
			payload: `{"base_url":"https://example.invalid","token_id":"operator@realm!token","token_secret":"synthetic-connector-sensitive-value","credential_id":"credential-profile-1"}`,
			handle:  (*Deps).HandleProxmoxConnectorTest,
		},
		{
			name:    "pbs",
			payload: `{"base_url":"https://example.invalid","token_id":"operator@realm!token","token_secret":"synthetic-connector-sensitive-value","credential_id":"credential-profile-1"}`,
			handle:  (*Deps).HandlePBSConnectorTest,
		},
		{
			name:    "truenas",
			payload: `{"base_url":"https://example.invalid","api_key":"synthetic-connector-sensitive-value","credential_id":"credential-profile-1"}`,
			handle:  (*Deps).HandleTrueNASConnectorTest,
		},
		{
			name:    "portainer",
			payload: `{"base_url":"https://example.invalid","token_secret":"synthetic-connector-sensitive-value","credential_id":"credential-profile-1"}`,
			handle:  (*Deps).HandlePortainerConnectorTest,
		},
		{
			name:    "homeassistant",
			payload: `{"base_url":"https://example.invalid","token":"synthetic-connector-sensitive-value","credential_id":"credential-profile-1"}`,
			handle:  (*Deps).HandleHomeAssistantConnectorTest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := &Deps{CredentialStore: credentialStore}
			ctx := apiv2.ContextWithScopes(context.Background(), []string{"collectors:write"})
			req := httptest.NewRequest(http.MethodPost, "/connectors/"+tt.name+"/test", strings.NewReader(tt.payload)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			tt.handle(deps, rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), secretValue) {
				t.Fatal("scope error reflected connector secret material")
			}
		})
	}
}
