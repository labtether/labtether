package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubcollector"
)

// TestProxmoxRoutesReachUpstreamThroughRouteTable sends one signed-in admin
// request to each /proxmox/ route through the real route table and auth, and
// checks that it reaches the configured Proxmox API with the stored token.
//
// The Proxmox handler tests live in internal/hubapi/proxmox and build their
// Deps with testutil/hubdeps. This test catches a fault in cmd's own wiring
// that those tests cannot see: a route bound to the wrong handler, or
// buildProxmoxDeps leaving out the collector store, credential store, secrets
// manager or asset store (the request then fails before the upstream call).
func TestProxmoxRoutesReachUpstreamThroughRouteTable(t *testing.T) {
	const tokenSecret = "wiring-token-secret"
	var mu sync.Mutex
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasSuffix(got, "="+tokenSecret) {
			t.Errorf("upstream %s: Authorization header does not carry the stored token", r.URL.Path)
		}
		mu.Lock()
		seen = append(seen, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/api2/json/cluster/ceph/status":
			_, _ = w.Write([]byte(`{"data":{}}`))
		case "/api2/json/cluster/resources":
			// Ceph pools are read per online node.
			_, _ = w.Write([]byte(`{"data":[{"type":"node","node":"pve01","status":"online"}]}`))
		default:
			_, _ = w.Write([]byte(`{"data":[]}`))
		}
	}))
	defer upstream.Close()

	sut := newTestAPIServer(t)
	createProxmoxCredentialProfile(t, sut, "cred-wiring", "labtether@pve!wiring", tokenSecret, upstream.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{collectors: []hubcollector.Collector{{
		ID: "collector-wiring", CollectorType: hubcollector.CollectorTypeProxmox, Enabled: true,
		Config: map[string]any{"base_url": upstream.URL, "token_id": "labtether@pve!wiring", "credential_id": "cred-wiring"},
	}}}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-101", Type: "vm", Name: "web-01", Source: "proxmox", Status: "online",
		Metadata: map[string]string{"proxmox_type": "qemu", "node": "pve01", "vmid": "101", "collector_id": "collector-wiring"},
	}); err != nil {
		t.Fatalf("seed Proxmox VM: %v", err)
	}
	admin := createRoleSessionToken(t, sut, "admin-proxmox-wiring", auth.RoleAdmin)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, tc := range []struct{ route, path, upstreamPath string }{
		{"/proxmox/assets/", "/proxmox/assets/proxmox-vm-101/firewall", "/api2/json/nodes/pve01/qemu/101/firewall/rules"},
		{"/proxmox/tasks/", "/proxmox/tasks/pve01/UPID-1/log", "/api2/json/nodes/pve01/tasks/UPID-1/log"},
		{"/proxmox/cluster/status", "/proxmox/cluster/status", "/api2/json/cluster/status"},
		{"/proxmox/cluster/resources", "/proxmox/cluster/resources", "/api2/json/cluster/resources"},
		{"/proxmox/nodes/", "/proxmox/nodes/pve01/network", "/api2/json/nodes/pve01/network"},
		{"/proxmox/ceph/", "/proxmox/ceph/pools", "/api2/json/cluster/resources"},
		{"/proxmox/ceph/status", "/proxmox/ceph/status", "/api2/json/cluster/ceph/status"},
	} {
		t.Run(tc.route, func(t *testing.T) {
			handler := handlers[tc.route]
			if handler == nil {
				t.Fatalf("route %s is not registered", tc.route)
			}
			mu.Lock()
			seen = nil
			mu.Unlock()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: admin})
			rec := httptest.NewRecorder()
			handler(rec, req)
			mu.Lock()
			defer mu.Unlock()
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s status=%d body=%s upstream=%v", tc.path, rec.Code, rec.Body.String(), seen)
			}
			if len(seen) == 0 || seen[0] != tc.upstreamPath {
				t.Fatalf("GET %s reached upstream paths %v, want first %s", tc.path, seen, tc.upstreamPath)
			}
		})
	}
}
