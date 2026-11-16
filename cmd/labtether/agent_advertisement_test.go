package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/servicehttp"
)

var explicitMonitoring = map[string]string{
	agentcaps.MetaMode: agentcaps.ModeExplicit, agentcaps.MetaEdition: agentcaps.EditionMonitoring,
	"cap_disks": "list", "cap_network": "list",
}

// Fault: a reboot is sent to an explicit agent that did not advertise power
// actions; the button waits for a result that never comes.
func TestV2PowerActionRefusesExplicitAgentWithoutPower(t *testing.T) {
	sut := newTestAPIServer(t)
	sender := installPowerResponder(t, sut, agentmgr.PowerResultAccepted, "")
	agentcaps.Record(sender.peer.Conn, explicitMonitoring)
	rec := httptest.NewRecorder()
	sut.handleV2AssetReboot(rec, powerRequest(http.MethodPost, "/api/v2/assets/node-1/reboot", nil, nil), "node-1")
	var body struct{ Error, Message string }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusUnprocessableEntity || body.Error != apiv2.AgentUnsupportedCode || body.Message != servicehttp.AgentUnsupportedMessage {
		t.Fatalf("got %d %s, want 422 agent_unsupported", rec.Code, rec.Body.String())
	}
	time.Sleep(100 * time.Millisecond)
	if n := sender.sends(); n != 0 {
		t.Fatalf("%d power action(s) sent", n)
	}

	agentcaps.Record(sender.peer.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, agentcaps.MetaCapabilities: "power"})
	rec = httptest.NewRecorder()
	sut.handleV2AssetReboot(rec, powerRequest(http.MethodPost, "/api/v2/assets/node-1/reboot", nil, nil), "node-1")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("advertised power: got %d %s, want 202", rec.Code, rec.Body.String())
	}
}

// Fault: an admin turns on the Hub SSH key for a device whose agent cannot
// add it; the setting is stored and never takes effect.
func TestHubSSHKeySettingRefusesExplicitAgentWithoutSSHKeys(t *testing.T) {
	server := newTestAPIServer(t)
	initializeTestHubSSHIdentity(t, server)
	if _, err := server.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-mon", Type: "host", Name: "node-mon", Source: "agent", Status: "online", Metadata: explicitMonitoring,
	}); err != nil {
		t.Fatal(err)
	}
	upsertHubSSHKeyTestAgent(t, server, "node-go")
	handlers := server.buildHTTPHandlers(nil, nil, nil)
	adminKey := createLegacyRouteAPIKeyWithRole(t, server, auth.RoleAdmin, []string{"hub:admin"}, nil)

	res := invokeLegacyRoute(t, handlers["/assets/"], http.MethodPut, "/assets/node-mon/hub-ssh-key", adminKey, `{"enabled":true}`)
	if res.Code != http.StatusUnprocessableEntity || server.hubSSHKeyEnabled("node-mon") {
		t.Fatalf("explicit agent: status=%d body=%s enabled=%v, want 422 and off", res.Code, res.Body.String(), server.hubSSHKeyEnabled("node-mon"))
	}
	if res := invokeLegacyRoute(t, handlers["/assets/"], http.MethodPut, "/assets/node-mon/hub-ssh-key", adminKey, `{"enabled":false}`); res.Code != http.StatusOK {
		t.Fatalf("turning it off: status=%d body=%s", res.Code, res.Body.String())
	}
	if res := invokeLegacyRoute(t, handlers["/assets/"], http.MethodPut, "/assets/node-go/hub-ssh-key", adminKey, `{"enabled":true}`); res.Code != http.StatusOK {
		t.Fatalf("agent without the signal: status=%d body=%s", res.Code, res.Body.String())
	}
}

// Fault: the metrics timers ask an explicit agent for processes and disks it
// never advertised, every interval, each one ending in a timeout.
func TestAgentTelemetryTimersSkipWhatIsNotAdvertised(t *testing.T) {
	peer := testutil.NewAgentPeer(t, testutil.ReplyDevice)
	agentcaps.Record(peer.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_network": "list"})
	_, adapter := newTelemetryFreezeAdapter(t, peer.Manager, 10*time.Millisecond)
	if adapter.collectProcesses(testutil.ReplyDevice, 5) != nil || adapter.collectDiskMounts(testutil.ReplyDevice) != nil {
		t.Fatal("collector returned entries")
	}
	peer.Silent(t, 100*time.Millisecond)
	go adapter.collectNetworkInterfaces(testutil.ReplyDevice)
	if got := peer.Next(t); got.Type != agentmgr.MsgNetworkList {
		t.Fatalf("sent %s, want the advertised %s", got.Type, agentmgr.MsgNetworkList)
	}
}

// Fault: a reconnecting explicit agent is sent the tmux probe and agent
// settings before its first heartbeat, because the connection starts empty.
func TestNewConnectionStartsWithTheStoredAdvertisement(t *testing.T) {
	server := newTestAPIServer(t)
	if _, err := server.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-mon", Type: "host", Name: "node-mon", Source: "agent", Metadata: explicitMonitoring,
	}); err != nil {
		t.Fatal(err)
	}
	conn := agentmgr.NewAgentConn(nil, "node-mon", "darwin")
	server.recordStoredAgentAdvertisement(conn)
	if agentcaps.ConnOffers(conn, agentmgr.MsgTerminalProbe) || !agentcaps.ConnOffers(conn, agentmgr.MsgDiskList) {
		t.Fatal("connection did not take the stored advertisement")
	}
}
