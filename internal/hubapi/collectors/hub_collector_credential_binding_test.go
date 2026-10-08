package collectors

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubcollector"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
