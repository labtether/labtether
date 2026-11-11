package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/policy"
)

// With Settings › Integration actions off, a Portainer container could still
// be stopped from its device page, a Home Assistant entity toggled, a Docker
// container stopped and a TrueNAS service started: only the Actions page
// checked the switch.
func TestIntegrationActionsOffRefusesEveryConnectorWrite(t *testing.T) {
	sut := newTestAPIServer(t)
	cfg := policy.DefaultEvaluatorConfig()
	cfg.ConnectorEnabled = false
	sut.policyState = newPolicyRuntimeState(cfg)

	for _, tc := range []struct {
		method, path string
		handle       http.HandlerFunc
	}{
		{http.MethodPost, "/portainer/assets/portainer-endpoint-1/containers/abc/stop", sut.handlePortainerAssets},
		{http.MethodPost, "/truenas/assets/truenas-host-1/services/smb/start", sut.handleTrueNASAssets},
		{http.MethodPost, "/pbs/assets/pbs-datastore-a/datastores/a/gc", sut.handlePBSAssets},
		{http.MethodPost, "/proxmox/assets/proxmox-vm-100/backup", sut.handleProxmoxAssets},
		{http.MethodPost, "/connectors/proxmox/actions/vm.start/execute", sut.handleConnectorActions},
		{http.MethodPost, "/api/v2/homeassistant/entities/ha-entity-light-desk", sut.handleV2HAEntityActions},
		{http.MethodPost, "/api/v1/docker/containers/docker-ct-a-1/action", sut.handleDockerContainerActions},
		// Objects named "test" are writes like any other, and only the exact
		// Test connection route is exempt.
		{http.MethodDelete, "/portainer/assets/portainer-endpoint-1/volumes/test", sut.handlePortainerAssets},
		{http.MethodDelete, "/truenas/assets/truenas-host-1/datasets/test", sut.handleTrueNASAssets},
		{http.MethodPost, "/connectors/proxmox/actions/vm.start/test", sut.handleConnectorActions},
		{http.MethodPost, "/connectors/proxmox%2Factions%2Fvm.start/test", sut.handleConnectorActions},
	} {
		rec := httptest.NewRecorder()
		tc.handle(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"action":"toggle"}`)))
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), integrationActionsOffMessage) {
			t.Errorf("%s %s with Integration actions off = %d %s, want 403 with the reason", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}

	// Reads and Test connection still work: they are answered by the handler, not refused here.
	for _, tc := range []struct {
		method, path string
		handle       http.HandlerFunc
	}{
		{http.MethodGet, "/portainer/assets/portainer-endpoint-1/containers", sut.handlePortainerAssets},
		{http.MethodPost, "/connectors/proxmox/test", sut.handleConnectorActions},
	} {
		rec := httptest.NewRecorder()
		tc.handle(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`)))
		if strings.Contains(rec.Body.String(), integrationActionsOffMessage) {
			t.Errorf("%s %s was refused by the switch: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}

	// Settings › Terminal sessions off closes a Portainer container shell too.
	shellOff := policy.DefaultEvaluatorConfig()
	shellOff.InteractiveEnabled = false
	shell := newTestAPIServer(t)
	shell.policyState = newPolicyRuntimeState(shellOff)
	rec := httptest.NewRecorder()
	shell.handlePortainerAssets(rec, httptest.NewRequest(http.MethodGet, "/portainer/assets/portainer-endpoint-1/containers/abc/exec", nil))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), terminalSessionsOffMessage) {
		t.Errorf("Portainer shell with Terminal sessions off = %d %s, want 403 with the reason", rec.Code, rec.Body.String())
	}

	if restart := sut.mcpRestartDockerContainer(); restart != nil {
		if err := restart(context.Background(), "docker-ct-a-1"); err != errIntegrationActionsOff {
			t.Errorf("MCP docker_container_restart with Integration actions off = %v, want %v", err, errIntegrationActionsOff)
		}
	}
}
