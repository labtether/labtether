package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// A failed service action used to reach the user as only "exit status 5":
// the Hub dropped the systemctl output the agent sent with it.
func TestFailedServiceActionSaysWhatSystemctlSaid(t *testing.T) {
	socket, peer, cleanup := createWoLWebSocketPair(t)
	defer cleanup()
	conn := agentmgr.NewAgentConn(socket, "srv-svc", "linux")
	manager := agentmgr.NewManager()
	manager.Register(conn)
	defer manager.Unregister(conn.AssetID)
	deps := &Deps{AgentMgr: manager, ServiceBridges: &sync.Map{}}
	go func() {
		_ = peer.SetReadDeadline(time.Now().Add(2 * time.Second))
		var msg agentmgr.Message
		if peer.ReadJSON(&msg) != nil {
			return
		}
		var req agentmgr.ServiceActionData
		_ = json.Unmarshal(msg.Data, &req)
		data, _ := json.Marshal(agentmgr.ServiceResultData{RequestID: req.RequestID, Error: "exit status 5",
			Output: "Failed to start ltqa-no-such.service: Unit ltqa-no-such.service not found.\n"})
		deps.ProcessAgentServiceResult(conn, agentmgr.Message{Type: agentmgr.MsgServiceResult, ID: req.RequestID, Data: data})
	}()
	rec := httptest.NewRecorder()
	deps.HandleServices(rec, httptest.NewRequest(http.MethodPost, "/services/srv-svc/start", strings.NewReader(`{"service":"ltqa-no-such.service"}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Unit ltqa-no-such.service not found.") ||
		strings.Contains(rec.Body.String(), "exit status") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

// apt-get prints progress lines first; its "E:" line is the reason. With no
// output at all the agent's error is all there is.
func TestAgentFailureMessagePicksTheErrorLine(t *testing.T) {
	apt := "Reading package lists...\nBuilding dependency tree...\nReading state information...\nE: Unable to locate package ltqa-x\n"
	if got := agentFailureMessage("exit status 100", apt); got != "E: Unable to locate package ltqa-x" {
		t.Fatalf("apt: got %q", got)
	}
	if got := agentFailureMessage("package action timed out", " \n"); got != "package action timed out" {
		t.Fatalf("no output: got %q", got)
	}
}
