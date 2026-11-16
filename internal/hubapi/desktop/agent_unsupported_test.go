package desktop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func remoteViewDeps(peer *testutil.AgentPeer) *Deps {
	return &Deps{
		AgentMgr:             peer.Manager,
		ClipboardReplies:     &ClipboardReplies{},
		DisplayReplies:       &DisplayReplies{},
		DiagnosticReplies:    &DiagnosticReplies{},
		GenerateRequestID:    func() string { return "req-1" },
		BrowserStreamTraceID: func(*http.Request) string { return "" },
		StreamTraceLogValue:  func(string) string { return "" },
	}
}

var remoteViewRoutes = []struct {
	name, sends string
	call        func(*Deps, http.ResponseWriter)
}{
	{"diagnose", agentmgr.MsgDesktopDiagnose, func(d *Deps, w http.ResponseWriter) {
		d.HandleDesktopDiagnoseRequest(w, httptest.NewRequest(http.MethodPost, "/desktop/diagnose/node01", nil))
	}},
	{"displays", agentmgr.MsgDesktopListDisplays, func(d *Deps, w http.ResponseWriter) {
		d.handleDisplayList(w, httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node01/displays", nil), "node01")
	}},
	{"clipboard get", agentmgr.MsgClipboardGet, func(d *Deps, w http.ResponseWriter) {
		d.HandleClipboardRoutes(w, httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node01/clipboard/get", strings.NewReader("{}")))
	}},
	{"clipboard set", agentmgr.MsgClipboardSet, func(d *Deps, w http.ResponseWriter) {
		d.HandleClipboardRoutes(w, httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node01/clipboard/set", strings.NewReader(`{"format":"text","text":"hi"}`)))
	}},
	{"desktop stream", "", func(d *Deps, w http.ResponseWriter) {
		d.HandleAgentDesktopStream(w, httptest.NewRequest(http.MethodGet, "/desktop/stream", nil), terminal.Session{ID: "s1", Target: "node01"})
	}},
	{"webrtc stream", "", func(d *Deps, w http.ResponseWriter) {
		d.HandleWebRTCStream(w, httptest.NewRequest(http.MethodGet, "/desktop/stream", nil), terminal.Session{ID: "s1", Target: "node01"})
	}},
	{"audio stream", "", func(d *Deps, w http.ResponseWriter) {
		d.HandleDesktopAudioStream(w, httptest.NewRequest(http.MethodGet, "/desktop/audio", nil), terminal.Session{ID: "s1", Target: "node01"})
	}},
}

// Fault: Remote View sends desktop, clipboard or WebRTC requests to an
// explicit agent that never advertised them, so the tab waits and times out.
func TestRemoteViewRefusesExplicitAgentWithoutSending(t *testing.T) {
	for _, route := range remoteViewRoutes {
		t.Run(route.name, func(t *testing.T) {
			peer := testutil.NewAgentPeer(t, "node01")
			agentcaps.Record(peer.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_disks": "list"})
			rec := httptest.NewRecorder()
			route.call(remoteViewDeps(peer), rec)
			var body struct{ Error string }
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != http.StatusUnprocessableEntity || body.Error != servicehttp.AgentUnsupportedMessage {
				t.Fatalf("got %d %s, want 422 %q", rec.Code, rec.Body.String(), servicehttp.AgentUnsupportedMessage)
			}
			peer.Silent(t, 150*time.Millisecond)
		})
	}
}

// Fault: the gate also stops an agent without the signal.
func TestRemoteViewStillReachesAgentWithoutTheSignal(t *testing.T) {
	for _, route := range remoteViewRoutes {
		t.Run(route.name, func(t *testing.T) {
			peer := testutil.NewAgentPeer(t, "node01")
			d := remoteViewDeps(peer)
			if route.sends == "" {
				// A stream route passes the gate and tries the WebSocket upgrade.
				rec := httptest.NewRecorder()
				route.call(d, rec)
				if rec.Code == http.StatusUnprocessableEntity {
					t.Fatalf("refused an agent without the signal: %s", rec.Body.String())
				}
				return
			}
			go route.call(d, httptest.NewRecorder())
			if got := peer.Next(t); got.Type != route.sends {
				t.Fatalf("sent %s, want %s", got.Type, route.sends)
			}
		})
	}
}

// Fault: an explicit agent that reports webrtc_available but did not
// advertise webrtc is routed to WebRTC instead of the VNC fallback.
func TestWebRTCNeedsTheAdvertisementForExplicitAgents(t *testing.T) {
	peer := testutil.NewAgentPeer(t, "node01")
	peer.Conn.SetMeta("webrtc_available", "true")
	d := remoteViewDeps(peer)
	if !d.ShouldUseWebRTC("node01") {
		t.Fatal("agent without the signal lost WebRTC")
	}
	agentcaps.Record(peer.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, agentcaps.MetaCapabilities: "desktop"})
	if d.ShouldUseWebRTC("node01") {
		t.Fatal("explicit agent without webrtc routed to WebRTC")
	}
}
