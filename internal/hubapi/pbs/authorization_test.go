package pbs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

func TestRestrictedPBSAssetRouteEnforcesAssetScope(t *testing.T) {
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	req := httptest.NewRequest(http.MethodGet, "/pbs/assets/asset-b/details", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	(&Deps{}).HandlePBSAssets(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRestrictedPBSStoreAccessCannotPivotToSiblingDatastore(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	for _, req := range []assets.HeartbeatRequest{
		{AssetID: "store-a", Source: "pbs", Type: "storage-pool", Metadata: map[string]string{"collector_id": "pbs-1", "store": "a"}},
		{AssetID: "store-b", Source: "pbs", Type: "storage-pool", Metadata: map[string]string{"collector_id": "pbs-1", "store": "b"}},
	} {
		if _, err := store.UpsertAssetHeartbeat(req); err != nil {
			t.Fatal(err)
		}
	}
	asset, ok, err := store.GetAsset("store-a")
	if err != nil || !ok {
		t.Fatalf("load asset: ok=%v err=%v", ok, err)
	}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"store-a"})
	req := httptest.NewRequest(http.MethodGet, "/pbs/assets/store-a/snapshots?store=b", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	if (&Deps{AssetStore: store}).requirePBSStoreAccess(rec, req, asset, "pbs-1", "b") {
		t.Fatal("expected sibling datastore access to be denied")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRestrictedPBSServerAggregateRequiresWholeCollector(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	for _, req := range []assets.HeartbeatRequest{
		{AssetID: "server", Source: "pbs", Type: "storage-controller", Metadata: map[string]string{"collector_id": "pbs-1"}},
		{AssetID: "store-a", Source: "pbs", Type: "storage-pool", Metadata: map[string]string{"collector_id": "pbs-1", "store": "a"}},
	} {
		if _, err := store.UpsertAssetHeartbeat(req); err != nil {
			t.Fatal(err)
		}
	}
	asset, _, _ := store.GetAsset("server")
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"server"})
	req := httptest.NewRequest(http.MethodGet, "/pbs/assets/server/details", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	if (&Deps{AssetStore: store}).requirePBSAggregateAccess(rec, req, asset, "pbs-1") {
		t.Fatal("expected partial collector aggregate access to be denied")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}
