package groupfeatures

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
)

func TestRestrictedGroupFeatureReadRejectsMixedGroupBeforeAggregating(t *testing.T) {
	groupStore := persistence.NewMemoryGroupStore()
	assetStore := persistence.NewMemoryAssetStore()
	group, err := groupStore.CreateGroup(groups.CreateRequest{Name: "Mixed"})
	if err != nil {
		t.Fatal(err)
	}
	for _, assetID := range []string{"asset-a", "asset-b"} {
		if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: assetID, Type: "server", Name: assetID, Source: "test"}); err != nil {
			t.Fatal(err)
		}
		if _, err := assetStore.UpdateAsset(assetID, assets.UpdateRequest{GroupID: &group.ID}); err != nil {
			t.Fatal(err)
		}
	}
	d := &Deps{GroupStore: groupStore, AssetStore: assetStore}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	req := httptest.NewRequest(http.MethodGet, "/groups/"+group.ID+"/timeline", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleGroupTimeline(rec, req, group.ID)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}
