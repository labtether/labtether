package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestV2ProviderMutationsRequireConnectorWriteScope(t *testing.T) {
	s := &apiServer{}
	for _, tc := range []struct {
		name    string
		path    string
		handler http.HandlerFunc
	}{
		{name: "proxmox asset", path: "/api/v2/proxmox/assets/a/firewall", handler: s.handleV2ProxmoxAssets},
		{name: "proxmox node", path: "/api/v2/proxmox/nodes/n/network", handler: s.handleV2ProxmoxNodeRoutes},
		{name: "proxmox task", path: "/api/v2/proxmox/tasks/n/u/stop", handler: s.handleV2ProxmoxTasks},
		{name: "truenas asset", path: "/api/v2/truenas/assets/a/snapshots", handler: s.handleV2TrueNASAssets},
		{name: "pbs asset", path: "/api/v2/pbs/assets/a/snapshots/forget", handler: s.handleV2PBSAssets},
		{name: "pbs task", path: "/api/v2/pbs/tasks/n/u/stop", handler: s.handleV2PBSTasks},
		{name: "portainer asset", path: "/api/v2/portainer/assets/a/containers/c/start", handler: s.handleV2PortainerAssets},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := contextWithScopes(context.Background(), []string{"connectors:read"})
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{}`)).WithContext(ctx)
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "connectors:write") {
				t.Fatalf("expected connectors:write denial, body=%s", rec.Body.String())
			}
		})
	}
}
