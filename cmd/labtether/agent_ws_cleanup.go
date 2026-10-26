package main

import (
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

// finishAgentWebSocketConnection owns disconnect side effects. Keep them in
// the same asset lifecycle transition as removal, so a reconnect cannot publish
// fresh state between the disconnected check and the cleanup sweep.
func (s *apiServer) finishAgentWebSocketConnection(conn *agentmgr.AgentConn, sessionID string) {
	if conn == nil {
		return
	}
	cleanup := func() { s.cleanupAgentConnectionState(conn.AssetID, sessionID) }

	if s.agentMgr == nil {
		cleanup()
		return
	}
	s.agentMgr.UnregisterIfMatch(conn.AssetID, conn)
}

// cleanupAgentConnectionState runs while registration for this asset is held.
func (s *apiServer) cleanupAgentConnectionState(assetID, sessionID string) {
	s.ensureResourcesDeps().FailAgentFileOperations(assetID)
	if s.broadcaster != nil {
		s.broadcaster.Broadcast("agent.disconnected", map[string]any{"asset_id": assetID})
	}
	s.closeWebRTCBridgesForAsset(assetID)
	s.closeDockerExecBridgesForAsset(assetID)
	s.closeDesktopBridgesForAsset(assetID)
	s.closeTerminalBridgesForAsset(assetID)
	if s.presenceStore != nil {
		deleted, err := s.presenceStore.DeletePresenceForSession(assetID, sessionID)
		if err != nil {
			securityruntime.Logf("agentws: presence delete failed for %s session=%s: %v", assetID, sessionID, err)
		}
		if deleted {
			_ = s.presenceStore.UpdateAssetTransportType(assetID, "offline")
		}
	}
	if s.dockerCoordinator != nil {
		s.dockerCoordinator.RemoveHost(assetID)
	}
	if s.webServiceCoordinator != nil {
		s.webServiceCoordinator.MarkHostDisconnected(assetID)
	}
}
