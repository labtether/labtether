package main

import (
	"errors"
	"net/http"
	"testing"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
)

// TestAdminRoutesReachHandlersThroughRouteTable sends one signed-in admin
// request to each admin route through the real route table and auth, and
// checks the answer only that route's handler gives.
//
// The admin handler tests call the handlers directly, and the route access
// pin records only status codes and errors, so two settings routes that both
// answer 200 could be swapped unseen. This test catches a fault in cmd's own
// wiring: an admin route bound to the wrong handler, or buildAdminDeps
// leaving out the runtime settings store, the TLS state, the reset store or
// the audit store. The protocol config handlers need Postgres and are not
// called here.
func TestAdminRoutesReachHandlersThroughRouteTable(t *testing.T) {
	origLookPath, origFallback := tailscaleLookPath, tailscaleFallbackPaths
	t.Cleanup(func() { tailscaleLookPath, tailscaleFallbackPaths = origLookPath, origFallback })
	tailscaleLookPath = func(string) (string, error) { return "", errors.New("tailscale is not installed in this test") }
	tailscaleFallbackPaths = func() []string { return nil }

	sut := newTestAPIServer(t)
	resets := persistence.NewMemoryAdminResetStore()
	sut.adminResetStore = resets
	sut.tlsState = TLSState{HttpsPort: 18443, HttpPort: 18080, CACertPEM: []byte("wiring CA certificate\n")}
	if _, err := sut.runtimeStore.SaveRuntimeSettingOverrides(map[string]string{runtimesettings.KeyConsoleDefaultActorID: "wiring-actor"}); err != nil {
		t.Fatalf("save runtime override: %v", err)
	}
	admin := createRoleSessionToken(t, sut, "admin-admin-wiring", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	call("/api/v1/ca.crt", http.MethodGet, "/api/v1/ca.crt", "", http.StatusOK, "wiring CA certificate")
	call("/api/v1/tls/info", http.MethodGet, "/api/v1/tls/info", "", http.StatusOK, `"https_port":18443`)
	call("/settings/tls", http.MethodGet, "/settings/tls", "", http.StatusOK, `"uploaded_override_present":false`)
	call("/settings/runtime", http.MethodGet, "/settings/runtime", "", http.StatusOK, "wiring-actor")
	call("/settings/runtime/reset", http.MethodPost, "/settings/runtime/reset", `{"keys":["`+runtimesettings.KeyConsoleDefaultActorID+`"]}`, http.StatusOK, "")
	overrides, err := sut.runtimeStore.ListRuntimeSettingOverrides()
	if err != nil || overrides[runtimesettings.KeyConsoleDefaultActorID] != "" {
		t.Fatalf("the reset did not reach the runtime settings store: overrides=%v err=%v", overrides, err)
	}
	call("/settings/managed-database", http.MethodGet, "/settings/managed-database", "", http.StatusOK, `"password_available":false`)
	call("/settings/managed-database/reveal", http.MethodPost, "/settings/managed-database/reveal", "", http.StatusNotFound, "managed database password unavailable")
	call("/settings/prometheus/test-connection", http.MethodPost, "/settings/prometheus/test-connection", `{`, http.StatusBadRequest,
		"invalid prometheus test connection payload")
	call("/settings/tailscale/serve", http.MethodGet, "/settings/tailscale/serve", "", http.StatusOK, `"serve_status":"not_installed"`)

	call("/admin/reset", http.MethodPost, "/admin/reset", `{"confirm":"RESET"}`, http.StatusOK, `"tables_cleared":38`)
	if !resets.ResetCalled {
		t.Fatal("the reset did not reach the reset store")
	}
	events, err := sut.auditStore.List(50, 0)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	for _, event := range events {
		if event.Type == "admin.reset" {
			return
		}
	}
	t.Fatalf("no admin.reset audit event in %d events", len(events))
}
