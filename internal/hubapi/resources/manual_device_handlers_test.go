package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
)

type failingManualDeviceExecer struct {
	err error
}

func (f failingManualDeviceExecer) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, f.err
}

type failingUpdateAssetStore struct {
	persistence.AssetStore
	err error
}

func (s *failingUpdateAssetStore) UpdateAsset(_ string, _ assets.UpdateRequest) (assets.Asset, error) {
	return assets.Asset{}, s.err
}

func TestHandleManualDeviceRoutesCleansUpAssetWhenHostUpdateFails(t *testing.T) {
	deps := newTestResourcesDeps(t)
	deps.ManualDeviceDB = failingManualDeviceExecer{err: errors.New("update failed")}

	req := httptest.NewRequest(http.MethodPost, "/assets/manual", bytes.NewBufferString(`{
		"name":"NAS",
		"host":"192.168.1.50"
	}`))
	rec := httptest.NewRecorder()
	deps.HandleManualDeviceRoutes(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
	}

	assetsList, err := deps.AssetStore.ListAssets()
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	if len(assetsList) != 0 {
		t.Fatalf("expected cleanup to remove partial asset, found %d assets", len(assetsList))
	}
}

func TestHandleManualDeviceRoutesCleansUpAssetWhenTagUpdateFails(t *testing.T) {
	deps := newTestResourcesDeps(t)
	deps.AssetStore = &failingUpdateAssetStore{
		AssetStore: testutil.NewAssetStore(),
		err:        errors.New("tag update failed"),
	}

	req := httptest.NewRequest(http.MethodPost, "/assets/manual", bytes.NewBufferString(`{
		"name":"NAS",
		"host":"192.168.1.50",
		"tags":["lab","nas"]
	}`))
	rec := httptest.NewRecorder()
	deps.HandleManualDeviceRoutes(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
	}

	assetsList, err := deps.AssetStore.ListAssets()
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	if len(assetsList) != 0 {
		t.Fatalf("expected cleanup to remove partial asset, found %d assets", len(assetsList))
	}
}

func TestHandleManualDeviceRoutesReturnsFinalAssetWithoutReload(t *testing.T) {
	deps := newTestResourcesDeps(t)

	req := httptest.NewRequest(http.MethodPost, "/assets/manual", bytes.NewBufferString(`{
		"name":"NAS",
		"host":"192.168.1.50",
		"tags":["lab","nas","lab"]
	}`))
	rec := httptest.NewRecorder()
	deps.HandleManualDeviceRoutes(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Asset assets.Asset `json:"asset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Asset.Host != "192.168.1.50" {
		t.Fatalf("expected host to be echoed in response, got %q", payload.Asset.Host)
	}
	if payload.Asset.TransportType != "manual" {
		t.Fatalf("expected transport_type=manual, got %q", payload.Asset.TransportType)
	}
	if len(payload.Asset.Tags) != 2 || payload.Asset.Tags[0] != "lab" || payload.Asset.Tags[1] != "nas" {
		t.Fatalf("expected normalized tags, got %#v", payload.Asset.Tags)
	}
}
