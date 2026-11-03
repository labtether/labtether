package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
)

// The agent keeps alerts in memory, and the Hub told it only when an alert
// started or ended, so after an agent restart or reconnect the Mac and
// Windows tray lost an alert that was still firing. A connecting agent must
// be told about its device's firing alerts, and only its device's.
func TestReplayAlertsToAgentSendsItsFiringAlerts(t *testing.T) {
	ctx := context.Background()
	ruleStore := persistence.NewMemoryAlertStore()
	instanceStore := persistence.NewMemoryAlertInstanceStore()
	rule, err := ruleStore.CreateAlertRule(alerts.CreateRuleRequest{Name: "Memory high", Description: "over 90 %", Severity: alerts.SeverityHigh})
	if err != nil {
		t.Fatal(err)
	}
	fire := func(fingerprint, assetID string) alerts.AlertInstance {
		t.Helper()
		inst, err := instanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: rule.ID, Fingerprint: fingerprint, Severity: rule.Severity})
		if err != nil {
			t.Fatal(err)
		}
		fired, err := instanceStore.FireAlertInstance(ctx, inst.ID, alerts.FiringScope{Version: 1, AssetIDs: []string{assetID}})
		if err != nil {
			t.Fatal(err)
		}
		return fired
	}
	mine := fire("node-a", "node-a")
	fire("node-b", "node-b")

	serverSide, agentSide := newReplaySocketPair(t)
	conn := agentmgr.NewAgentConn(serverSide, "node-a", "linux")
	deps := &Deps{AlertStore: ruleStore, AlertInstanceStore: instanceStore}
	deps.ReplayAlertsToAgent(ctx, conn)

	_ = agentSide.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := agentSide.ReadJSON(&msg); err != nil {
		t.Fatalf("agent got no alert after connecting: %v", err)
	}
	var data agentmgr.AlertNotifyData
	_ = json.Unmarshal(msg.Data, &data)
	if msg.Type != agentmgr.MsgAlertNotify || data.ID != mine.ID || data.State != "firing" || data.Title != "Memory high" {
		t.Fatalf("got %s %+v, want firing %s", msg.Type, data, mine.ID)
	}
	_ = agentSide.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if err := agentSide.ReadJSON(&msg); err == nil {
		t.Fatalf("agent also got another device's alert: %s", msg.Data)
	}
}

func newReplaySocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	serverConns := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			serverConns <- conn
		}
	}))
	t.Cleanup(server.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	select {
	case conn := <-serverConns:
		t.Cleanup(func() { _ = conn.Close() })
		return conn, client
	case <-time.After(2 * time.Second):
		t.Fatal("no server websocket")
	}
	return nil, nil
}
