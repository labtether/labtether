package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

func enableTestHubSSHKey(t *testing.T, server *apiServer, assetID string) {
	t.Helper()
	if _, err := server.hubSSHKeyStore.SetHubSSHKeySetting(assetID, true, false); err != nil {
		t.Fatalf("turn on Hub SSH key for %s: %v", assetID, err)
	}
}

func upsertHubSSHKeyTestAgent(t *testing.T, server *apiServer, assetID string) assets.Asset {
	t.Helper()
	asset, err := server.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Type: "host", Name: assetID, Source: "agent", Status: "online",
	})
	if err != nil {
		t.Fatalf("upsert agent asset %s: %v", assetID, err)
	}
	return asset
}

func connectHubSSHKeyTestAgent(t *testing.T, server *apiServer, assetID string) (*agentmgr.AgentConn, *websocket.Conn) {
	t.Helper()
	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	t.Cleanup(cleanup)
	conn := agentmgr.NewAgentConn(serverConn, assetID, "linux")
	server.agentMgr.Register(conn)
	t.Cleanup(func() { server.agentMgr.Unregister(assetID) })
	return conn, clientConn
}

// readRawHubSSHKeyPayload returns the exact authorized_keys line in the next
// message, which must have type wantType.
func readRawHubSSHKeyPayload(t *testing.T, conn *websocket.Conn, wantType string) string {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var message agentmgr.Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("read %s: %v", wantType, err)
	}
	if message.Type != wantType {
		t.Fatalf("message type=%q, want %q", message.Type, wantType)
	}
	var payload struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(message.Data, &payload); err != nil {
		t.Fatalf("decode %s: %v", wantType, err)
	}
	return payload.PublicKey
}

// A device enrolled after the upgrade must not get the Hub's root SSH key, or
// an SSH login config, until an admin turns it on. Turning it off again must
// remove both key lines the Hub may have added.
func TestHubSSHKeyStartsOffAndOnlyAnAdminTurnsItOn(t *testing.T) {
	server := newTestAPIServer(t)
	identity := initializeTestHubSSHIdentity(t, server)
	upsertHubSSHKeyTestAgent(t, server, "node-new")
	conn, agent := connectHubSSHKeyTestAgent(t, server, "node-new")

	server.syncHubSSHKeyOnConnect(conn)
	if err := conn.Send(agentmgr.Message{Type: "test.marker"}); err != nil {
		t.Fatalf("send marker: %v", err)
	}
	if err := agent.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var first agentmgr.Message
	if err := agent.ReadJSON(&first); err != nil || first.Type != "test.marker" {
		t.Fatalf("a new device received %q on connect (err=%v); want no Hub SSH key", first.Type, err)
	}
	installed := marshalAgentMessage(t, agentmgr.SSHKeyInstalledData{Username: "root", Hostname: "node-new"})
	conn.SetMeta("ssh_source_ip", "192.0.2.10") // a usable SSH address, so only the setting stops it
	server.processAgentSSHKeyInstalled(conn, installed)
	if _, ok, _ := server.credentialStore.GetAssetTerminalConfig("node-new"); ok {
		t.Fatal("an unsolicited ssh_key.installed created an SSH login config for a device with the key off")
	}

	handlers := server.buildHTTPHandlers(nil, nil, nil)
	path := "/assets/node-new/hub-ssh-key"
	for name, key := range map[string]string{
		"operator":            createLegacyRouteAPIKeyWithRole(t, server, auth.RoleOperator, []string{"hub:admin"}, nil),
		"admin without scope": createLegacyRouteAPIKeyWithRole(t, server, auth.RoleAdmin, []string{"assets:write"}, nil),
	} {
		if res := invokeLegacyRoute(t, handlers["/assets/"], http.MethodPut, path, key, `{"enabled":true}`); res.Code != http.StatusForbidden {
			t.Fatalf("%s turning the key on: status=%d body=%s", name, res.Code, res.Body.String())
		}
	}
	if server.hubSSHKeyEnabled("node-new") {
		t.Fatal("a refused request turned the Hub SSH key on")
	}

	adminKey := createLegacyRouteAPIKeyWithRole(t, server, auth.RoleAdmin, []string{"hub:admin"}, nil)
	if res := invokeLegacyRoute(t, handlers["/assets/"], http.MethodPut, path, adminKey, `{"enabled":true}`); res.Code != http.StatusOK {
		t.Fatalf("admin turning the key on: status=%d body=%s", res.Code, res.Body.String())
	}
	plain := strings.TrimSpace(identity.PublicKey)
	marked := shared.HubAuthorizedKeyLine(identity.PublicKey)
	if got := readRawHubSSHKeyPayload(t, agent, agentmgr.MsgSSHKeyInstall); got != marked {
		t.Fatalf("installed line=%q, want %q", got, marked)
	}

	if res := invokeLegacyRoute(t, handlers["/assets/"], http.MethodPut, path, adminKey, `{"enabled":false}`); res.Code != http.StatusOK {
		t.Fatalf("admin turning the key off: status=%d body=%s", res.Code, res.Body.String())
	}
	for _, want := range []string{plain, marked} {
		if got := readRawHubSSHKeyPayload(t, agent, agentmgr.MsgSSHKeyRemove); got != want {
			t.Fatalf("removed line=%q, want %q", got, want)
		}
	}
	if setting, err := server.hubSSHKeyStore.GetHubSSHKeySetting("node-new"); err != nil || setting.Enabled || setting.RemovalPending {
		t.Fatalf("setting after turning off=%+v err=%v", setting, err)
	}

	events, err := server.auditStore.List(20, 0)
	if err != nil {
		t.Fatal(err)
	}
	changes := 0
	for _, event := range events {
		if event.Type == hubSSHKeySettingAuditType {
			changes++
			if event.Target != "node-new" || !strings.HasPrefix(event.ActorID, "apikey:") {
				t.Fatalf("audit event missing device or actor: %+v", event)
			}
		}
	}
	if changes != 2 {
		t.Fatalf("Hub SSH key audit events=%d, want 2", changes)
	}
}

// Turning the key off while the agent is offline must still remove it: the
// removal waits and is sent when the agent next connects.
func TestHubSSHKeyRemovalWaitsForAnOfflineAgent(t *testing.T) {
	server := newTestAPIServer(t)
	identity := initializeTestHubSSHIdentity(t, server)
	asset := upsertHubSSHKeyTestAgent(t, server, "node-offline")
	enableTestHubSSHKey(t, server, "node-offline")

	change, err := server.applyHubSSHKeySetting(asset, false)
	if err != nil || !change.Setting.RemovalPending || change.RemovalSent {
		t.Fatalf("turning off an offline device: change=%+v err=%v", change, err)
	}

	conn, agent := connectHubSSHKeyTestAgent(t, server, "node-offline")
	server.syncHubSSHKeyOnConnect(conn)
	for _, want := range []string{strings.TrimSpace(identity.PublicKey), shared.HubAuthorizedKeyLine(identity.PublicKey)} {
		if got := readRawHubSSHKeyPayload(t, agent, agentmgr.MsgSSHKeyRemove); got != want {
			t.Fatalf("removed line=%q, want %q", got, want)
		}
	}
	if setting, err := server.hubSSHKeyStore.GetHubSSHKeySetting("node-offline"); err != nil || setting.Enabled || setting.RemovalPending {
		t.Fatalf("setting after reconnect=%+v err=%v", setting, err)
	}
}
