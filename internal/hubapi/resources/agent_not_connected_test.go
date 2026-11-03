package resources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// An offline device used to answer 502, and every client shows a 5xx message
// as "An internal error occurred." The answer must say the agent is not
// connected (409), and an ID that names no device must be a 404.
func TestDeviceRoutesSayAgentIsNotConnected(t *testing.T) {
	deps := &Deps{AssetStore: testutil.NewAssetStore(), AgentMgr: agentmgr.NewManager()}
	if _, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv-offline", Type: "host", Name: "srv-offline", Source: "agent", Status: "online", Platform: "linux",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	routes := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
	}{
		{"disks", deps.HandleDisks, http.MethodGet, "/disks/%s"},
		{"packages", deps.HandlePackages, http.MethodGet, "/packages/%s"},
		{"package install", deps.HandlePackages, http.MethodPost, "/packages/%s/install"},
		{"services", deps.HandleServices, http.MethodGet, "/services/%s"},
		{"network", deps.HandleNetworks, http.MethodGet, "/network/%s"},
	}
	for _, route := range routes {
		for _, device := range []struct {
			id       string
			wantCode int
			wantText string
		}{
			{"srv-offline", http.StatusConflict, "agent is not connected (last seen: "},
			{"srv-missing", http.StatusNotFound, "device not found"},
		} {
			t.Run(route.name+"/"+device.id, func(t *testing.T) {
				rec := httptest.NewRecorder()
				route.handler(rec, httptest.NewRequest(route.method, fmt.Sprintf(route.path, device.id), nil))
				var body struct {
					Error string `json:"error"`
				}
				_ = json.Unmarshal(rec.Body.Bytes(), &body)
				if rec.Code != device.wantCode || !strings.HasPrefix(body.Error, device.wantText) {
					t.Fatalf("got %d %q, want %d starting %q", rec.Code, body.Error, device.wantCode, device.wantText)
				}
			})
		}
	}
}
