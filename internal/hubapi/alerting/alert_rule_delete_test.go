package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

// Deleting a rule while its alert fired left the alert "firing" in the Mac
// menu bar and the Windows tray until the app restarted: the Hub deleted the
// alert without telling the agent. The agent must get "resolved".
func TestDeletingAFiringRuleTellsTheAgentItEnded(t *testing.T) {
	ctx := context.Background()
	assetStore := persistence.NewMemoryAssetStore()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "node-a", Type: "host", Source: "agent", Status: "online"}); err != nil {
		t.Fatal(err)
	}
	ruleStore := persistence.NewMemoryAlertStore()
	instanceStore := persistence.NewMemoryAlertInstanceStore()
	rule, err := ruleStore.CreateAlertRule(alerts.CreateRuleRequest{
		Name: "Memory high", Severity: alerts.SeverityCritical,
		TargetScope: alerts.TargetScopeAsset, Targets: []alerts.RuleTargetInput{{AssetID: "node-a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	inst, err := instanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: rule.ID, Fingerprint: "node-a", Severity: rule.Severity})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instanceStore.FireAlertInstance(ctx, inst.ID, alerts.FiringScope{Version: 1, AssetIDs: []string{"node-a"}}); err != nil {
		t.Fatal(err)
	}

	serverSide, agentSide := newReplaySocketPair(t)
	mgr := agentmgr.NewManager()
	mgr.Register(agentmgr.NewAgentConn(serverSide, "node-a", "darwin"))
	deps := &Deps{AlertStore: ruleStore, AlertInstanceStore: instanceStore, AssetStore: assetStore, AgentMgr: mgr}

	rec := httptest.NewRecorder()
	deps.HandleAlertRuleActions(rec, httptest.NewRequest(http.MethodDelete, "/alerts/rules/"+rule.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete answered %d: %s", rec.Code, rec.Body.String())
	}

	_ = agentSide.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := agentSide.ReadJSON(&msg); err != nil {
		t.Fatalf("agent was not told the alert ended: %v", err)
	}
	var data agentmgr.AlertNotifyData
	_ = json.Unmarshal(msg.Data, &data)
	if msg.Type != agentmgr.MsgAlertNotify || data.ID != inst.ID || data.State != "resolved" {
		t.Fatalf("got %s %+v, want resolved %s", msg.Type, data, inst.ID)
	}
}
