package alerting

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
)

// A rule that watches two devices told both agents "firing" when one
// breached, so the Mac and Windows tray showed an alert on a healthy
// machine. Only the device that breaches may be told; a device that joins
// later is told then; one that recovers, or is away when the alert ends,
// is told "resolved".
func TestAlertPushGoesOnlyToBreachingDevices(t *testing.T) {
	ctx := context.Background()
	rule := alerts.Rule{ID: "rule-1", Name: "Memory high", Severity: alerts.SeverityHigh}
	mgr := agentmgr.NewManager()
	// A websocket read that times out breaks the socket, so each agent side
	// is read by its own goroutine into a channel.
	received := map[string]chan string{"node-a": make(chan string, 8), "node-b": make(chan string, 8)}
	connect := func(id string) *agentmgr.AgentConn {
		serverSide, agentSide := newReplaySocketPair(t)
		conn := agentmgr.NewAgentConn(serverSide, id, "linux")
		mgr.Register(conn)
		go func(socket *websocket.Conn, out chan<- string) {
			for {
				var msg agentmgr.Message
				if err := socket.ReadJSON(&msg); err != nil {
					return
				}
				var data agentmgr.AlertNotifyData
				_ = json.Unmarshal(msg.Data, &data)
				out <- data.ID + " " + data.State
			}
		}(agentSide, received[id])
		return conn
	}
	connect("node-a")
	connect("node-b")
	deps := &Deps{AgentMgr: mgr, AgentAlertDevices: NewAgentAlertDevices(),
		AlertStore: persistence.NewMemoryAlertStore(), AlertInstanceStore: persistence.NewMemoryAlertInstanceStore()}
	got := func(id string) string {
		select {
		case message := <-received[id]:
			return message
		case <-time.After(300 * time.Millisecond):
			return ""
		}
	}
	expect := func(step, a, b string) {
		t.Helper()
		if gotA, gotB := got("node-a"), got("node-b"); gotA != a || gotB != b {
			t.Fatalf("%s: node-a got %q, node-b got %q; want %q and %q", step, gotA, gotB, a, b)
		}
	}
	scope := func(ids ...string) alerts.FiringScope { return alerts.FiringScope{Version: 1, AssetIDs: ids} }
	inst := alerts.AlertInstance{ID: "alert-1", RuleID: rule.ID}

	deps.pushAlertFiringToAgents(ctx, rule, inst.ID, scope("node-a"))
	expect("node-a breaches", "alert-1 firing", "")

	deps.syncAlertFiringDevices(ctx, rule, inst, scope("node-a"))
	expect("still only node-a", "", "")

	deps.syncAlertFiringDevices(ctx, rule, inst, scope("node-a", "node-b"))
	expect("node-b breaches too", "", "alert-1 firing")

	deps.syncAlertFiringDevices(ctx, rule, inst, scope("node-b"))
	expect("node-a recovers", "alert-1 resolved", "")

	// node-b's agent drops off, the alert ends, the agent comes back.
	mgr.Unregister("node-b")
	deps.pushAlertResolvedToAgents(ctx, rule, inst.ID)
	expect("alert ends while node-b is away", "", "")
	deps.ReplayAlertsToAgent(ctx, connect("node-b"))
	expect("node-b reconnects", "", "alert-1 resolved")
}
