package truenas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

func TestRestrictedTrueNASAssetRouteEnforcesAssetScope(t *testing.T) {
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	req := httptest.NewRequest(http.MethodGet, "/truenas/assets/asset-b/overview", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	(&Deps{}).HandleTrueNASAssets(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRestrictedTrueNASCollectorWideActionRequiresAllSiblingAssets(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	for _, req := range []assets.HeartbeatRequest{
		{AssetID: "asset-a", Source: "truenas", Metadata: map[string]string{"collector_id": "tn-1"}},
		{AssetID: "asset-b", Source: "truenas", Metadata: map[string]string{"collector_id": "tn-1"}},
	} {
		if _, err := store.UpsertAssetHeartbeat(req); err != nil {
			t.Fatal(err)
		}
	}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	req := httptest.NewRequest(http.MethodGet, "/truenas/assets/asset-a/overview", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	(&Deps{AssetStore: store}).HandleTrueNASAssets(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}
