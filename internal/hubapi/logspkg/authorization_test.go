package logspkg

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

func TestRestrictedLogViewsFilterGlobalAndSecretViews(t *testing.T) {
	store := persistence.NewMemoryLogStore()
	for _, request := range []logs.SavedViewRequest{
		{Name: "allowed", AssetID: "asset-a"},
		{Name: "secret", AssetID: "asset-b"},
		{Name: "global"},
	} {
		if _, err := store.SaveView("system", request); err != nil {
			t.Fatal(err)
		}
	}
	d := &Deps{LogStore: store}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	req := httptest.NewRequest(http.MethodGet, "/logs/views", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleLogViews(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Views []logs.SavedView `json:"views"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Views) != 1 || response.Views[0].AssetID != "asset-a" {
		t.Fatalf("unexpected filtered views: %#v", response.Views)
	}
}

func TestRestrictedLogViewUpdateCannotClearOrSwitchAsset(t *testing.T) {
	store := persistence.NewMemoryLogStore()
	view, err := store.SaveView("system", logs.SavedViewRequest{Name: "allowed", AssetID: "asset-a"})
	if err != nil {
		t.Fatal(err)
	}
	d := &Deps{LogStore: store}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	req := httptest.NewRequest(http.MethodPatch, "/logs/views/"+view.ID, bytes.NewReader([]byte(`{"name":"escaped","asset_id":"asset-b"}`))).WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleLogViewActions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	stored, ok, err := store.GetView("system", view.ID)
	if err != nil || !ok {
		t.Fatalf("reload view: ok=%v err=%v", ok, err)
	}
	if stored.AssetID != "asset-a" || stored.Name != "allowed" {
		t.Fatalf("forbidden update reached store: %#v", stored)
	}
}
