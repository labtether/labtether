package main

import (
	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
)

// recordStoredAgentAdvertisement gives a new connection the stored device's
// last advertisement. The Hub sends a terminal probe, the Hub SSH key and
// agent settings on connect, before the first heartbeat arrives.
func (s *apiServer) recordStoredAgentAdvertisement(conn *agentmgr.AgentConn) {
	if s.assetStore == nil || conn == nil {
		return
	}
	stored, ok, err := s.assetStore.GetAsset(conn.AssetID)
	if err != nil || !ok {
		return
	}
	agentcaps.Record(conn, stored.Metadata)
}
