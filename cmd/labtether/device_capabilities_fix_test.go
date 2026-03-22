package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestHandleTrueNASDisksSmartTestRequiresAdmin(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/truenas/assets/truenas-host/disks/sda/smart-test", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "operator-1", "operator"))
	rec := httptest.NewRecorder()

	sut.handleTrueNASDisks(context.Background(), rec, req, assets.Asset{ID: "truenas-host"}, nil, []string{"sda", "smart-test"})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin smart test, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}

func TestHandlePortainerContainerExecRequiresAdmin(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/portainer/assets/portainer-host/containers/abc123/exec", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "operator-1", "operator"))
	rec := httptest.NewRecorder()

	sut.handlePortainerContainerExec(rec, req, "portainer-host", "abc123")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin portainer exec, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}

func TestHandlePortainerContainerExecRequiresWebSocketUpgrade(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/portainer/assets/portainer-host/containers/abc123/exec", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec := httptest.NewRecorder()

	sut.handlePortainerContainerExec(rec, req, "portainer-host", "abc123")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without websocket upgrade, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}

func TestHandlePortainerCapabilitiesReportsExecSupportFromAuthMethod(t *testing.T) {
	tests := []struct {
		name        string
		authMethod  string
		wantCanExec bool
	}{
		{name: "defaults api key to no exec", authMethod: "", wantCanExec: false},
		{name: "password auth enables exec", authMethod: "password", wantCanExec: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sut := newTestAPIServer(t)
			sut.hubCollectorStore = &stubHubCollectorStore{
				collectors: []hubcollector.Collector{
					{
						ID:            "collector-portainer-1",
						CollectorType: hubcollector.CollectorTypePortainer,
						Enabled:       true,
						Config: map[string]any{
							"auth_method": tt.authMethod,
						},
					},
				},
			}

			rec := httptest.NewRecorder()
			sut.handlePortainerCapabilities(rec, assets.Asset{
				ID:     "portainer-host-1",
				Source: "portainer",
				Type:   "container-host",
				Metadata: map[string]string{
					"collector_id": "collector-portainer-1",
				},
			})

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
			}

			var payload portainerCapabilities
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode capabilities: %v", err)
			}
			if payload.CanExec != tt.wantCanExec {
				t.Fatalf("CanExec = %v, want %v", payload.CanExec, tt.wantCanExec)
			}
		})
	}
}

func TestHandleTrueNASCapabilitiesKeepsProbeErrorsConservative(t *testing.T) {
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "vm.query":
			return nil, &trueNASRPCError{Code: -32000, Message: "temporary failure"}
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	createTrueNASCredentialProfile(t, sut, "cred-truenas-capabilities", "api-key-1", server.URL)
	configureTrueNASCollectors(t, sut, hubcollector.Collector{
		ID:            "collector-truenas-capabilities",
		AssetID:       "truenas-cluster-1",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-capabilities",
			"skip_verify":   true,
		},
	})

	asset := assets.Asset{
		ID:       "truenas-host-omeganas",
		Name:     "omeganas",
		Source:   "truenas",
		Type:     "nas",
		Metadata: map[string]string{"collector_id": "collector-truenas-capabilities"},
	}
	runtime, err := sut.loadTrueNASRuntime("collector-truenas-capabilities")
	if err != nil {
		t.Fatalf("loadTrueNASRuntime failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/capabilities", nil)
	rec := httptest.NewRecorder()
	sut.handleTrueNASCapabilities(req.Context(), rec, asset, runtime)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}

	var payload truenasCapabilities
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if payload.IsScale {
		t.Fatalf("expected transient vm.query failure to keep IsScale false, got true")
	}
	if len(payload.Warnings) == 0 {
		t.Fatalf("expected detection warning when vm.query fails, got none")
	}
}

func TestBuildProxmoxCapabilityTabsOmitsStorageHAAndReplication(t *testing.T) {
	tabs := buildProxmoxCapabilityTabs("storage")
	allowed := make(map[string]bool, len(tabs))
	for _, tab := range tabs {
		allowed[tab] = true
	}
	if allowed["ha"] {
		t.Fatalf("storage capabilities unexpectedly exposed ha tab: %+v", tabs)
	}
	if allowed["replication"] {
		t.Fatalf("storage capabilities unexpectedly exposed replication tab: %+v", tabs)
	}
}

func TestSelectProxmoxHADoesNotMatchStorageAssets(t *testing.T) {
	match, related := selectProxmoxHA([]proxmox.HAResource{
		{SID: "vm:101", Node: "pve01", State: "started"},
		{SID: "ct:202", Node: "pve01", State: "started"},
	}, proxmoxSessionTarget{Kind: "storage", Node: "pve01"})

	if match != nil {
		t.Fatalf("expected no ha match for storage asset, got %+v", match)
	}
	if len(related) != 0 {
		t.Fatalf("expected no related ha resources for storage asset, got %d", len(related))
	}
}
