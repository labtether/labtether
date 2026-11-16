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

// TestPBSRoutesReachUpstreamThroughRouteTable sends one signed-in admin
// request to each /pbs/ route through the real route table and auth, and
// checks that it reaches the configured PBS API with the stored token.
//
// The PBS handler tests live in internal/hubapi/pbs and build their Deps
// with testutil/hubdeps. This test catches a fault in cmd's own wiring that
// those tests cannot see: a route bound to the wrong handler, or
// buildPBSDeps (or ensurePBSDeps) leaving out the collector store,
// credential store, secrets manager or asset store (the request then fails
// before the upstream call).
func TestPBSRoutesReachUpstreamThroughRouteTable(t *testing.T) {
	const tokenSecret = "wiring-token-secret"
	var mu sync.Mutex
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasSuffix(got, ":"+tokenSecret) {
			t.Errorf("upstream %s: Authorization header does not carry the stored token", r.URL.Path)
		}
		mu.Lock()
		seen = append(seen, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/api2/json/admin/datastore":
			_, _ = w.Write([]byte(`{"data":[{"store":"backup"}]}`))
		case "/api2/json/nodes/localhost/tasks/UPID-1/status":
			_, _ = w.Write([]byte(`{"data":{"upid":"UPID-1","status":"stopped"}}`))
		default:
			_, _ = w.Write([]byte(`{"data":[]}`))
		}
	}))
	defer upstream.Close()

	sut := newTestAPIServer(t)
	createPBSCredentialProfile(t, sut, "cred-wiring", "root@pam!wiring", tokenSecret, upstream.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{collectors: []hubcollector.Collector{{
		ID: "collector-wiring", CollectorType: hubcollector.CollectorTypePBS, Enabled: true,
		Config: map[string]any{"base_url": upstream.URL, "token_id": "root@pam!wiring", "credential_id": "cred-wiring"},
	}}}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-backup", Type: "storage-pool", Name: "backup", Source: "pbs", Status: "online",
		Metadata: map[string]string{"store": "backup", "node": "localhost", "collector_id": "collector-wiring"},
	}); err != nil {
		t.Fatalf("seed PBS datastore: %v", err)
	}
	admin := createRoleSessionToken(t, sut, "admin-pbs-wiring", auth.RoleAdmin)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, tc := range []struct{ route, path, upstreamPath string }{
		{"/pbs/assets/", "/pbs/assets/pbs-datastore-backup/snapshots", "/api2/json/admin/datastore/backup/snapshots"},
		{"/pbs/tasks/", "/pbs/tasks/localhost/UPID-1/status", "/api2/json/nodes/localhost/tasks/UPID-1/status"},
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
			found := false
			for _, p := range seen {
				found = found || p == tc.upstreamPath
			}
			if !found {
				t.Fatalf("GET %s reached upstream paths %v, want %s", tc.path, seen, tc.upstreamPath)
			}
		})
	}
}
