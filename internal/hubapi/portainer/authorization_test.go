package portainer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

func TestRestrictedPortainerRoutesEnforceAssetScope(t *testing.T) {
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	assetReq := httptest.NewRequest(http.MethodGet, "/portainer/assets/asset-b/overview", nil).WithContext(ctx)
	assetRec := httptest.NewRecorder()
	(&Deps{}).HandlePortainerAssets(assetRec, assetReq)
	if assetRec.Code != http.StatusForbidden {
		t.Fatalf("asset route: expected 403, got %d body=%s", assetRec.Code, assetRec.Body.String())
	}

	globalReq := httptest.NewRequest(http.MethodGet, "/portainer/endpoints", nil).WithContext(ctx)
	globalRec := httptest.NewRecorder()
	(&Deps{}).HandlePortainerEndpoints(globalRec, globalReq)
	if globalRec.Code != http.StatusForbidden {
		t.Fatalf("global route: expected 403, got %d body=%s", globalRec.Code, globalRec.Body.String())
	}
}

func TestPortainerExecRequiresConnectorWriteScope(t *testing.T) {
	d := &Deps{RequireAdminAuth: func(http.ResponseWriter, *http.Request) bool { return true }}
	ctx := apiv2.ContextWithScopes(context.Background(), []string{"connectors:read"})
	req := httptest.NewRequest(http.MethodGet, "/portainer/assets/asset-a/containers/container-a/exec", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	d.HandlePortainerContainerExec(rec, req, "asset-a", "container-a")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRestrictedPortainerChildResourcesAreIndependentlyAuthorized(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	for _, req := range []assets.HeartbeatRequest{
		{AssetID: "host-a", Type: "container-host", Name: "host", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1"}},
		{AssetID: "container-a", Type: "container", Name: "allowed", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1", "container_id": "container-id-a"}},
		{AssetID: "container-b", Type: "container", Name: "secret", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1", "container_id": "container-id-b"}},
	} {
		if _, err := store.UpsertAssetHeartbeat(req); err != nil {
			t.Fatal(err)
		}
	}
	d := &Deps{AssetStore: store}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"host-a", "container-a"})

	if allowed, err := d.portainerResourceAllowed(ctx, "1", "container_id", "container-id-a"); err != nil || !allowed {
		t.Fatalf("expected allowed child resource: allowed=%v err=%v", allowed, err)
	}
	if allowed, err := d.portainerResourceAllowed(ctx, "1", "container_id", "container-id-b"); err != nil || allowed {
		t.Fatalf("expected secret child resource to be denied: allowed=%v err=%v", allowed, err)
	}
	if allowed, err := d.portainerEndpointAllowed(ctx, "1"); err != nil || allowed {
		t.Fatalf("expected mixed endpoint-wide operations to be denied: allowed=%v err=%v", allowed, err)
	}
}
