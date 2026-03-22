package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
)

func TestHandleCronListBridgesAgentInventory(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-cron", "linux"))
	defer sut.agentMgr.Unregister("node-cron")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgCronList {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgCronList)
			return
		}

		var req agentmgr.CronListData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode cron list payload: %v", err)
			return
		}

		raw, _ := json.Marshal(agentmgr.CronListedData{
			RequestID: req.RequestID,
			Entries: []agentmgr.CronEntry{{
				Source:   "crontab",
				Schedule: "*/5 * * * *",
				Command:  "/usr/local/bin/backup",
				User:     "root",
			}},
		})
		sut.processAgentCronListed(&agentmgr.AgentConn{AssetID: "node-cron"}, agentmgr.Message{
			Type: agentmgr.MsgCronListed,
			ID:   req.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodGet, "/cron/node-cron", nil)
	rec := httptest.NewRecorder()
	sut.handleCrons(rec, req)

	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.CronListedData
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Entries) != 1 || response.Entries[0].Command != "/usr/local/bin/backup" {
		t.Fatalf("unexpected response %+v", response)
	}
}

func TestHandleUsersListBridgesAgentInventory(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-users", "linux"))
	defer sut.agentMgr.Unregister("node-users")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgUsersList {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgUsersList)
			return
		}

		var req agentmgr.UsersListData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode users list payload: %v", err)
			return
		}

		raw, _ := json.Marshal(agentmgr.UsersListedData{
			RequestID: req.RequestID,
			Sessions: []agentmgr.UserSession{{
				Username:   "alice",
				Terminal:   "pts/0",
				RemoteHost: "10.0.0.5",
				LoginTime:  "2026-03-08T14:30:00Z",
			}},
		})
		sut.processAgentUsersListed(&agentmgr.AgentConn{AssetID: "node-users"}, agentmgr.Message{
			Type: agentmgr.MsgUsersListed,
			ID:   req.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodGet, "/users/node-users", nil)
	rec := httptest.NewRecorder()
	sut.handleUsers(rec, req)

	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.UsersListedData
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 1 || response.Sessions[0].Username != "alice" {
		t.Fatalf("unexpected response %+v", response)
	}
}

func TestHandleDiskListBridgesAgentInventory(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-disk", "linux"))
	defer sut.agentMgr.Unregister("node-disk")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgDiskList {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgDiskList)
			return
		}

		var req agentmgr.DiskListData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode disk list payload: %v", err)
			return
		}

		raw, _ := json.Marshal(agentmgr.DiskListedData{
			RequestID: req.RequestID,
			Mounts: []agentmgr.MountInfo{{
				Device:     "/dev/sda1",
				MountPoint: "/",
				FSType:     "ext4",
				Total:      100,
				Used:       40,
				Available:  60,
				UsePct:     40,
			}},
		})
		sut.processAgentDiskListed(&agentmgr.AgentConn{AssetID: "node-disk"}, agentmgr.Message{
			Type: agentmgr.MsgDiskListed,
			ID:   req.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodGet, "/disks/node-disk", nil)
	rec := httptest.NewRecorder()
	sut.handleDisks(rec, req)

	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.DiskListedData
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Mounts) != 1 || response.Mounts[0].MountPoint != "/" {
		t.Fatalf("unexpected response %+v", response)
	}
}

func TestSendSSHKeyInstallAndRemoveEmitHubKey(t *testing.T) {
	var sut apiServer
	sut.hubIdentity = &hubSSHIdentity{
		ProfileID: "cred-hub",
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHUB hub",
	}

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	conn := agentmgr.NewAgentConn(serverConn, "node-ssh", "linux")
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))

	readPayload := func(wantType string) string {
		t.Helper()
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Fatalf("read outbound %s: %v", wantType, err)
		}
		if outbound.Type != wantType {
			t.Fatalf("outbound type=%q, want %q", outbound.Type, wantType)
		}

		var payload agentmgr.SSHKeyInstallData
		if err := json.Unmarshal(outbound.Data, &payload); err != nil {
			t.Fatalf("decode %s payload: %v", wantType, err)
		}
		return payload.PublicKey
	}

	sut.sendSSHKeyInstall(conn)
	if got := readPayload(agentmgr.MsgSSHKeyInstall); got != sut.hubIdentity.PublicKey {
		t.Fatalf("install public key=%q, want %q", got, sut.hubIdentity.PublicKey)
	}

	sut.sendSSHKeyRemove(conn)
	if got := readPayload(agentmgr.MsgSSHKeyRemove); got != sut.hubIdentity.PublicKey {
		t.Fatalf("remove public key=%q, want %q", got, sut.hubIdentity.PublicKey)
	}
}

func TestProcessAgentSSHKeyInstalledSavesTerminalConfigWhenMissing(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.hubIdentity = &hubSSHIdentity{ProfileID: "cred-hub"}

	msg := marshalAgentMessage(t, agentmgr.SSHKeyInstalledData{
		Username: "labuser",
		Hostname: "lab-host",
	})
	sut.processAgentSSHKeyInstalled(&agentmgr.AgentConn{AssetID: "node-ssh"}, msg)

	cfg, ok, err := sut.credentialStore.GetAssetTerminalConfig("node-ssh")
	if err != nil {
		t.Fatalf("load terminal config: %v", err)
	}
	if !ok {
		t.Fatalf("expected terminal config to be saved")
	}
	if cfg.Host != "lab-host" || cfg.Port != 22 || cfg.Username != "labuser" || cfg.CredentialProfileID != "cred-hub" || !cfg.StrictHostKey {
		t.Fatalf("unexpected terminal config %+v", cfg)
	}
}

func TestProcessAgentSSHKeyInstalledPreservesExistingTerminalConfig(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.hubIdentity = &hubSSHIdentity{ProfileID: "cred-hub"}

	existing, err := sut.credentialStore.SaveAssetTerminalConfig(credentials.AssetTerminalConfig{
		AssetID:             "node-ssh",
		Host:                "custom-host",
		Port:                2222,
		Username:            "owner",
		StrictHostKey:       false,
		CredentialProfileID: "cred-custom",
	})
	if err != nil {
		t.Fatalf("save existing terminal config: %v", err)
	}

	msg := marshalAgentMessage(t, agentmgr.SSHKeyInstalledData{
		Username: "labuser",
		Hostname: "lab-host",
	})
	sut.processAgentSSHKeyInstalled(&agentmgr.AgentConn{AssetID: "node-ssh"}, msg)

	cfg, ok, err := sut.credentialStore.GetAssetTerminalConfig("node-ssh")
	if err != nil {
		t.Fatalf("load terminal config: %v", err)
	}
	if !ok {
		t.Fatalf("expected terminal config to exist")
	}
	if cfg.Host != existing.Host || cfg.Port != existing.Port || cfg.Username != existing.Username || cfg.CredentialProfileID != existing.CredentialProfileID || cfg.StrictHostKey != existing.StrictHostKey {
		t.Fatalf("terminal config was overwritten: got %+v want %+v", cfg, existing)
	}
}

func TestHandleWakeOnLANDirectSendUsesFallbackWhenNoRelay(t *testing.T) {
	sut := newTestAPIServer(t)
	mustSeedWakeAsset(t, sut, "sleepy-node", map[string]string{"mac_address": "aa:bb:cc:dd:ee:ff"})

	originalSendWakeOnLAN := respkg.SendWakeOnLAN
	t.Cleanup(func() { respkg.SendWakeOnLAN = originalSendWakeOnLAN })

	var (
		gotMAC       string
		gotBroadcast string
	)
	respkg.SendWakeOnLAN = func(mac net.HardwareAddr, broadcastAddr string) error {
		gotMAC = mac.String()
		gotBroadcast = broadcastAddr
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/assets/sleepy-node/wake", nil)
	rec := httptest.NewRecorder()
	sut.handleWakeOnLAN(rec, req, "sleepy-node")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotMAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("sent MAC=%q, want aa:bb:cc:dd:ee:ff", gotMAC)
	}
	if gotBroadcast != "255.255.255.255:9" {
		t.Fatalf("broadcast=%q, want 255.255.255.255:9", gotBroadcast)
	}
}

func TestHandleWakeOnLANUsesFirstValidMACCandidate(t *testing.T) {
	sut := newTestAPIServer(t)
	mustSeedWakeAsset(t, sut, "sleepy-node", map[string]string{
		"mac_address": "not-a-mac",
		"primary_mac": "aa:bb:cc:dd:ee:ff",
	})

	originalSendWakeOnLAN := respkg.SendWakeOnLAN
	t.Cleanup(func() { respkg.SendWakeOnLAN = originalSendWakeOnLAN })

	var gotMAC string
	respkg.SendWakeOnLAN = func(mac net.HardwareAddr, _ string) error {
		gotMAC = mac.String()
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/assets/sleepy-node/wake", nil)
	rec := httptest.NewRecorder()
	sut.handleWakeOnLAN(rec, req, "sleepy-node")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotMAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("sent MAC=%q, want aa:bb:cc:dd:ee:ff", gotMAC)
	}
}

func TestHandleWakeOnLANRejectsInvalidMACWhenNoValidFallbackExists(t *testing.T) {
	sut := newTestAPIServer(t)
	mustSeedWakeAsset(t, sut, "sleepy-node", map[string]string{"mac_address": "not-a-mac"})

	req := httptest.NewRequest(http.MethodPost, "/assets/sleepy-node/wake", nil)
	rec := httptest.NewRecorder()
	sut.handleWakeOnLAN(rec, req, "sleepy-node")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleWakeOnLANPrefersAgentRelayWhenAvailable(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	mustSeedWakeAsset(t, sut, "target-node", map[string]string{"mac_address": "aa:bb:cc:dd:ee:ff"})
	mustSeedWakeAsset(t, sut, "relay-node", map[string]string{"mac_address": "11:22:33:44:55:66"})

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "relay-node", "linux"))
	defer sut.agentMgr.Unregister("relay-node")

	done := make(chan struct{})
	go func() {
		defer close(done)
		clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgWoLSend {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgWoLSend)
			return
		}
		if outbound.ID != "target-node" {
			t.Errorf("outbound id=%q, want target-node", outbound.ID)
		}

		var payload agentmgr.WoLSendData
		if err := json.Unmarshal(outbound.Data, &payload); err != nil {
			t.Errorf("decode wol payload: %v", err)
			return
		}
		if payload.MAC != "aa:bb:cc:dd:ee:ff" || payload.Broadcast != "255.255.255.255:9" || payload.RequestID == "" {
			t.Errorf("unexpected wol payload %+v", payload)
		}
	}()

	req := httptest.NewRequest(http.MethodPost, "/assets/target-node/wake", nil)
	rec := httptest.NewRecorder()
	sut.handleWakeOnLAN(rec, req, "target-node")

	<-done

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["method"] != "agent-assisted" {
		t.Fatalf("method=%q, want agent-assisted", response["method"])
	}
}

func TestHandleWakeOnLANPrefersLinuxRelayOverOtherPlatforms(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	mustSeedWakeAsset(t, sut, "target-node", map[string]string{"mac_address": "aa:bb:cc:dd:ee:ff"})
	mustSeedWakeAssetWithPlatform(t, sut, "relay-darwin", "darwin", map[string]string{"mac_address": "11:22:33:44:55:66"})
	mustSeedWakeAssetWithPlatform(t, sut, "relay-linux", "linux", map[string]string{"mac_address": "22:33:44:55:66:77"})

	darwinServerConn, darwinClientConn, darwinCleanup := createWSPairForNetworkTest(t)
	defer darwinCleanup()
	linuxServerConn, linuxClientConn, linuxCleanup := createWSPairForNetworkTest(t)
	defer linuxCleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(darwinServerConn, "relay-darwin", "darwin"))
	defer sut.agentMgr.Unregister("relay-darwin")
	sut.agentMgr.Register(agentmgr.NewAgentConn(linuxServerConn, "relay-linux", "linux"))
	defer sut.agentMgr.Unregister("relay-linux")

	req := httptest.NewRequest(http.MethodPost, "/assets/target-node/wake", nil)
	rec := httptest.NewRecorder()
	sut.handleWakeOnLAN(rec, req, "target-node")

	assertWakeRelayMessage(t, linuxClientConn, "target-node")
	assertNoAgentMessage(t, darwinClientConn)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["method"] != "agent-assisted" {
		t.Fatalf("method=%q, want agent-assisted", response["method"])
	}
}

func TestHandleWakeOnLANUsesStableRelayIDOrderingWithinPlatform(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	mustSeedWakeAsset(t, sut, "target-node", map[string]string{"mac_address": "aa:bb:cc:dd:ee:ff"})
	mustSeedWakeAssetWithPlatform(t, sut, "relay-z", "linux", map[string]string{"mac_address": "11:22:33:44:55:66"})
	mustSeedWakeAssetWithPlatform(t, sut, "relay-a", "linux", map[string]string{"mac_address": "22:33:44:55:66:77"})

	serverConnZ, clientConnZ, cleanupZ := createWSPairForNetworkTest(t)
	defer cleanupZ()
	serverConnA, clientConnA, cleanupA := createWSPairForNetworkTest(t)
	defer cleanupA()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConnZ, "relay-z", "linux"))
	defer sut.agentMgr.Unregister("relay-z")
	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConnA, "relay-a", "linux"))
	defer sut.agentMgr.Unregister("relay-a")

	req := httptest.NewRequest(http.MethodPost, "/assets/target-node/wake", nil)
	rec := httptest.NewRecorder()
	sut.handleWakeOnLAN(rec, req, "target-node")

	assertWakeRelayMessage(t, clientConnA, "target-node")
	assertNoAgentMessage(t, clientConnZ)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func mustSeedWakeAsset(t *testing.T, sut *apiServer, assetID string, metadata map[string]string) {
	t.Helper()
	mustSeedWakeAssetWithPlatform(t, sut, assetID, "linux", metadata)
}

func mustSeedWakeAssetWithPlatform(t *testing.T, sut *apiServer, assetID, platform string, metadata map[string]string) {
	t.Helper()

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  assetID,
		Type:     "host",
		Name:     assetID,
		Source:   "agent",
		Platform: platform,
		Metadata: metadata,
	}); err != nil {
		t.Fatalf("upsert wake asset %s: %v", assetID, err)
	}
}

func assertWakeRelayMessage(t *testing.T, conn *websocket.Conn, targetAssetID string) {
	t.Helper()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var outbound agentmgr.Message
	if err := conn.ReadJSON(&outbound); err != nil {
		t.Fatalf("read outbound: %v", err)
	}
	if outbound.Type != agentmgr.MsgWoLSend {
		t.Fatalf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgWoLSend)
	}
	if outbound.ID != targetAssetID {
		t.Fatalf("outbound id=%q, want %q", outbound.ID, targetAssetID)
	}

	var payload agentmgr.WoLSendData
	if err := json.Unmarshal(outbound.Data, &payload); err != nil {
		t.Fatalf("decode wol payload: %v", err)
	}
	if payload.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("payload MAC=%q, want aa:bb:cc:dd:ee:ff", payload.MAC)
	}
	if payload.Broadcast != "255.255.255.255:9" {
		t.Fatalf("payload broadcast=%q, want 255.255.255.255:9", payload.Broadcast)
	}
	if payload.RequestID == "" {
		t.Fatal("expected non-empty wol request id")
	}
}

func assertNoAgentMessage(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	var outbound agentmgr.Message
	if err := conn.ReadJSON(&outbound); err == nil {
		t.Fatalf("unexpected outbound message %+v", outbound)
	}
}
