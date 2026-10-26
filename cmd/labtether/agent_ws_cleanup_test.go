package main

import (
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
)

type disconnectPresenceStore struct {
	persistence.PresenceStore
	deletedSession string
	offlineAsset   string
}

func (s *disconnectPresenceStore) DeletePresenceForSession(assetID, sessionID string) (bool, error) {
	s.deletedSession = sessionID
	return true, nil
}

func (s *disconnectPresenceStore) UpdateAssetTransportType(assetID, transport string) error {
	if transport == "offline" {
		s.offlineAsset = assetID
	}
	return nil
}

func disconnectTestBridges(s *apiServer, assetID string) []<-chan struct{} {
	terminalClosed := make(chan struct{})
	desktopClosed := make(chan struct{})
	webrtcClosed := make(chan struct{})
	dockerClosed := make(chan struct{})
	file := newFileBridge(1, assetID)
	s.terminalBridges.Store("terminal", &terminalBridge{ExpectedAgentID: assetID, ClosedCh: terminalClosed})
	s.desktopBridges.Store("desktop", &desktopBridge{ExpectedAgentID: assetID, ClosedCh: desktopClosed})
	s.webrtcBridges.Store("webrtc", &webrtcSignalingBridge{ExpectedAgentID: assetID, ClosedCh: webrtcClosed})
	s.dockerExecBridges.Store("docker", &dockerExecBridge{ExpectedAgentID: assetID, ClosedCh: dockerClosed})
	s.fileBridges.Store("file", file)
	return []<-chan struct{}{terminalClosed, desktopClosed, webrtcClosed, dockerClosed, file.Done}
}

func TestAgentDisconnectCleanupAfterUnregister(t *testing.T) {
	serverConn, _, cleanup := newWebSocketPair(t)
	defer cleanup()
	conn := agentmgr.NewAgentConn(serverConn, "node", "linux")
	presence := &disconnectPresenceStore{}
	s := &apiServer{agentMgr: agentmgr.NewManager(), presenceStore: presence}
	s.agentMgr.RegisterWithLifecycle(conn, func(owner *agentmgr.AgentConn) {
		s.cleanupAgentConnectionState(owner.AssetID, "old-presence")
	}, nil)
	bridges := disconnectTestBridges(s, "node")
	s.agentMgr.UnregisterIfMatch("node", conn)
	s.finishAgentWebSocketConnection(conn, "old-presence")
	for i, closed := range bridges {
		select {
		case <-closed:
		default:
			t.Fatalf("bridge %d remained open after early unregistration", i)
		}
	}
	if presence.deletedSession != "old-presence" || presence.offlineAsset != "node" {
		t.Fatalf("presence cleanup missing: %+v", presence)
	}
}

func TestAgentDisconnectCleanupPreservesReplacement(t *testing.T) {
	oldSocket, _, oldCleanup := newWebSocketPair(t)
	defer oldCleanup()
	newSocket, _, newCleanup := newWebSocketPair(t)
	defer newCleanup()
	old := agentmgr.NewAgentConn(oldSocket, "node", "linux")
	newer := agentmgr.NewAgentConn(newSocket, "node", "linux")
	presence := &disconnectPresenceStore{}
	s := &apiServer{agentMgr: agentmgr.NewManager(), presenceStore: presence}
	s.agentMgr.RegisterWithLifecycle(old, func(owner *agentmgr.AgentConn) {
		s.cleanupAgentConnectionState(owner.AssetID, "old-presence")
	}, nil)
	oldBridges := disconnectTestBridges(s, "node")
	// Ping, token revocation or heartbeat rejection can remove the old socket
	// before its deferred read-loop cleanup runs and before a reconnect.
	s.agentMgr.UnregisterIfMatch(old.AssetID, old)
	s.agentMgr.Register(newer)
	for i, closed := range oldBridges {
		select {
		case <-closed:
		default:
			t.Fatalf("old bridge %d survived replacement", i)
		}
	}
	presence.deletedSession = ""
	presence.offlineAsset = ""
	newBridges := disconnectTestBridges(s, "node")
	s.finishAgentWebSocketConnection(old, "old-presence")
	for i, closed := range newBridges {
		select {
		case <-closed:
			t.Fatalf("replacement bridge %d was closed by old cleanup", i)
		default:
		}
	}
	if presence.deletedSession != "" || presence.offlineAsset != "" {
		t.Fatal("old cleanup changed replacement presence")
	}
}
