package proxmox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

func TestRestrictedProxmoxRoutesEnforceAssetScopeAndDenyGlobalViews(t *testing.T) {
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	d := &Deps{}

	assetReq := httptest.NewRequest(http.MethodGet, "/proxmox/assets/asset-b/details", nil).WithContext(ctx)
	assetRec := httptest.NewRecorder()
	d.HandleProxmoxAssets(assetRec, assetReq)
	if assetRec.Code != http.StatusForbidden {
		t.Fatalf("asset route: expected 403, got %d body=%s", assetRec.Code, assetRec.Body.String())
	}

	globalReq := httptest.NewRequest(http.MethodGet, "/proxmox/cluster/status", nil).WithContext(ctx)
	globalRec := httptest.NewRecorder()
	d.HandleProxmoxClusterStatus(globalRec, globalReq)
	if globalRec.Code != http.StatusForbidden {
		t.Fatalf("global route: expected 403, got %d body=%s", globalRec.Code, globalRec.Body.String())
	}
}

func TestRestrictedProxmoxAggregateAssetViewRequiresWholeCollector(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	for _, req := range []assets.HeartbeatRequest{
		{AssetID: "vm-a", Source: "proxmox", Type: "vm", Metadata: map[string]string{"collector_id": "pve-1", "proxmox_type": "qemu", "node": "pve", "vmid": "100"}},
		{AssetID: "vm-b", Source: "proxmox", Type: "vm", Metadata: map[string]string{"collector_id": "pve-1", "proxmox_type": "qemu", "node": "pve", "vmid": "101"}},
	} {
		if _, err := store.UpsertAssetHeartbeat(req); err != nil {
			t.Fatal(err)
		}
	}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"vm-a"})
	req := httptest.NewRequest(http.MethodGet, "/proxmox/assets/vm-a/details", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	(&Deps{AssetStore: store}).HandleProxmoxAssets(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}
