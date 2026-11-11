package resources

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

// Renaming a device to "nas01\r\nBcc: x@example.com" was stored as given. A
// typed name with a line break or other control character is refused in
// words, on edit and on create, and the stored name stays.
func TestDeviceNameWithControlCharactersIsRefused(t *testing.T) {
	d := newTestResourcesDeps(t)
	if _, err := d.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "nas01", Type: "server", Name: "nas01", Source: "test"}); err != nil {
		t.Fatalf("create asset: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/assets/nas01", strings.NewReader(`{"name":"nas01\r\nBcc: x@example.com"}`))
	d.HandleAssetActions(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "can't contain line breaks") {
		t.Fatalf("rename = %d %s, want 400 in words", rec.Code, rec.Body.String())
	}
	if asset, _, _ := d.AssetStore.GetAsset("nas01"); asset.Name != "nas01" {
		t.Fatalf("stored name = %q, want it unchanged", asset.Name)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v2/assets", strings.NewReader(`{"name":"nas02\tbox"}`))
	d.V2CreateAsset(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "can't contain line breaks") {
		t.Fatalf("create = %d %s, want 400 in words", rec.Code, rec.Body.String())
	}
}
