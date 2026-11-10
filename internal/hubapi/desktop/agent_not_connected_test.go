package desktop

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

// Remote View clipboard, audio and stream routes answered 502 for an agent
// that is not connected, which users see as "An internal error occurred."
// They must answer like the other device routes: 409 with the last-seen
// time, or 404 for an ID that names no device.
func TestRemoteViewRoutesSayAgentIsNotConnected(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "srv-offline", Status: "online"}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	deps := &Deps{AssetStore: store, AgentMgr: agentmgr.NewManager()}
	routes := map[string]func(http.ResponseWriter, string){
		"clipboard": func(w http.ResponseWriter, id string) {
			deps.HandleClipboardRoutes(w, httptest.NewRequest(http.MethodPost, "/api/v1/nodes/"+id+"/clipboard/get", strings.NewReader("{}")))
		},
		"audio": func(w http.ResponseWriter, id string) {
			deps.HandleDesktopAudioStream(w, httptest.NewRequest(http.MethodGet, "/desktop/audio", nil), terminal.Session{ID: "s1", Target: id})
		},
	}
	for name, call := range routes {
		for id, want := range map[string]struct {
			code int
			text string
		}{
			"srv-offline": {http.StatusConflict, "agent is not connected (last seen: "},
			"srv-missing": {http.StatusNotFound, "device not found"},
		} {
			rec := httptest.NewRecorder()
			call(rec, id)
			if rec.Code != want.code || !strings.Contains(rec.Body.String(), want.text) {
				t.Fatalf("%s %s: got %d %s, want %d %q", name, id, rec.Code, rec.Body.String(), want.code, want.text)
			}
		}
	}
}
