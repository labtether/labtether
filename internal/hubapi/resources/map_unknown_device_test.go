package resources

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// The Map routes answered 200 with an empty list for an ID that names no
// device, which reads as "this device has no links". One-device requests
// must say "device not found" (404). A real device without links stays 200
// and empty, and the console's batch form (asset_ids) is unchanged: an ID
// that no longer exists only adds nothing.
func TestMapRoutesTellUnknownDeviceFromNoLinks(t *testing.T) {
	deps := &Deps{EdgeStore: &authzEdgeStore{}, AssetStore: testutil.NewAssetStore()}
	if _, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "srv-known", Status: "online"}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	cases := []struct {
		name, path string
		handler    http.HandlerFunc
		wantCode   int
	}{
		{"connections, unknown", "/edges?asset_id=srv-missing", deps.HandleEdges, http.StatusNotFound},
		{"connections, known", "/edges?asset_id=srv-known", deps.HandleEdges, http.StatusOK},
		{"console batch with a stale ID", "/edges?asset_ids=srv-known,srv-missing", deps.HandleEdges, http.StatusOK},
		{"console batch of one stale ID", "/edges?asset_ids=srv-missing", deps.HandleEdges, http.StatusOK},
		{"downstream, unknown", "/edges/tree?root=srv-missing", deps.HandleEdgeByID, http.StatusNotFound},
		{"downstream, known", "/edges/tree?root=srv-known", deps.HandleEdgeByID, http.StatusOK},
		{"upstream, unknown", "/edges/ancestors?id=srv-missing", deps.HandleEdgeByID, http.StatusNotFound},
		{"upstream, known", "/edges/ancestors?id=srv-known", deps.HandleEdgeByID, http.StatusOK},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		tc.handler(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.wantCode {
			t.Fatalf("%s: got %d %s, want %d", tc.name, rec.Code, rec.Body.String(), tc.wantCode)
		}
		if tc.wantCode == http.StatusNotFound && !strings.Contains(rec.Body.String(), "device not found") {
			t.Fatalf("%s: got %s, want device not found", tc.name, rec.Body.String())
		}
	}
}
