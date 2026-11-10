package desktop

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

// rdpGatewayOnLoopback points this Hub's RDP gateway at a loopback port.
func rdpGatewayOnLoopback(t *testing.T, port int) {
	t.Helper()
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	t.Setenv("GUACD_HOST", "127.0.0.1")
	t.Setenv("GUACD_PORT", strconv.Itoa(port))
}

// loopbackPortWithNoListener is a port nothing listens on: a Hub whose
// gateway was never started.
func loopbackPortWithNoListener(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release the port: %v", err)
	}
	return port
}

func errorText(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body.Error
}

// A default install runs no RDP gateway. The stream answered 502 "guacd
// unavailable", which reached people as "An internal error occurred." and
// left the console on "Opening stream" for ever. The stream ticket and the
// stream must both say why in plain words, and the ticket must not leave a
// session behind that can never work.
func TestRDPIsRefusedInPlainWordsOnAHubWithoutItsGateway(t *testing.T) {
	rdpGatewayOnLoopback(t, loopbackPortWithNoListener(t))
	const want = "RDP needs the Hub's RDP gateway (guacd), which is not running on this Hub."

	store := persistence.NewMemoryTerminalStore()
	session, err := store.CreateSession(terminal.CreateSessionRequest{ActorID: "owner", Target: "192.0.2.10:3389", Mode: "desktop"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	var optionsMu, spiceMu sync.RWMutex
	options := map[string]DesktopSessionOptions{
		session.ID: {Protocol: "rdp", Direct: true, DirectHost: "192.0.2.10", DirectPort: 3389},
	}
	spice := map[string]DesktopSPICEProxyTarget{}
	deps := &Deps{
		TerminalStore:    store,
		DesktopSessionMu: &optionsMu, DesktopSessionOpts: &options,
		DesktopSPICEMu: &spiceMu, DesktopSPICE: &spice,
		EnforceRateLimit: func(http.ResponseWriter, *http.Request, string, int, time.Duration) bool { return true },
	}

	rec := httptest.NewRecorder()
	deps.HandleGuacdDesktopStream(rec, httptest.NewRequest(http.MethodGet, "/desktop/sessions/"+session.ID+"/stream", nil), session)
	if got := errorText(t, rec); rec.Code != http.StatusConflict || got != want {
		t.Fatalf("stream: got %d %q, want 409 %q", rec.Code, got, want)
	}

	rec = httptest.NewRecorder()
	deps.HandleDesktopStreamTicket(rec, httptest.NewRequest(http.MethodPost, "/desktop/sessions/"+session.ID+"/stream-ticket", nil), session)
	if got := errorText(t, rec); rec.Code != http.StatusConflict || got != want {
		t.Fatalf("stream ticket: got %d %q, want 409 %q", rec.Code, got, want)
	}
	if _, open, _ := store.GetSession(session.ID); open {
		t.Fatal("the session stayed open after its stream ticket was refused")
	}
}

// The console offered "RDP (Recommended)" for every Windows device, also on a
// Hub that cannot serve RDP. The displays answer, which the Remote View panel
// already loads, must say whether the gateway is there: false without it, and
// true with it (or RDP would never be offered).
func TestDisplaysAnswerSaysWhetherThisHubCanServeRDP(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	accepted := make(chan *websocket.Conn, 1)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ws, err := upgrader.Upgrade(w, r, nil); err == nil {
			accepted <- ws
		}
	}))
	defer agentServer.Close()
	agentSide := desktopTestSocket(t, agentServer.URL)
	var hubSide *websocket.Conn
	select {
	case hubSide = <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("agent did not connect")
	}
	conn := agentmgr.NewAgentConn(hubSide, "win01", "windows")
	defer conn.Close()
	manager := agentmgr.NewManager()
	manager.Register(conn)
	var bridges sync.Map
	deps := &Deps{AgentMgr: manager, DisplayBridges: &bridges}

	gateway, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start a gateway: %v", err)
	}
	defer gateway.Close()

	for _, tc := range []struct {
		name string
		port int
		want bool
	}{
		{"gateway running", gateway.Addr().(*net.TCPAddr).Port, true},
		{"no gateway", loopbackPortWithNoListener(t), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rdpGatewayOnLoopback(t, tc.port)
			rec := httptest.NewRecorder()
			answered := make(chan struct{})
			go func() {
				defer close(answered)
				deps.handleDisplayList(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nodes/win01/displays", nil), "win01")
			}()

			// The agent's side: read the request and answer it.
			_ = agentSide.SetReadDeadline(time.Now().Add(2 * time.Second))
			var request agentmgr.Message
			if err := agentSide.ReadJSON(&request); err != nil {
				t.Fatalf("agent got no display request: %v", err)
			}
			var asked struct {
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(request.Data, &asked); err != nil {
				t.Fatalf("decode display request: %v", err)
			}
			answer, _ := json.Marshal(agentmgr.DisplayListData{RequestID: asked.RequestID})
			deps.ProcessAgentDesktopDisplays(conn, agentmgr.Message{Type: agentmgr.MsgDesktopDisplays, Data: answer})

			select {
			case <-answered:
			case <-time.After(5 * time.Second):
				t.Fatal("displays did not answer")
			}
			var body struct {
				RDPGateway *bool `json:"rdp_gateway"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("displays: %d %s (%v)", rec.Code, rec.Body.String(), err)
			}
			if body.RDPGateway == nil || *body.RDPGateway != tc.want {
				t.Fatalf("rdp_gateway in %s, want %t", rec.Body.String(), tc.want)
			}
		})
	}
}
