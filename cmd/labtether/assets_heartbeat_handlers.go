package main

// assets_heartbeat_handlers.go — thin forwarding stubs for heartbeat and
// asset-delete handlers. All business logic has been extracted to
// internal/hubapi/resources (heartbeat_handlers.go, delete_handlers.go,
// asset_helpers.go). cmd/labtether is a pure composition root.

import (
	"github.com/labtether/labtether/internal/assets"
)

// processHeartbeatRequest forwards to the resources package implementation.
// It is called from the agent WebSocket handler (agent_ws_handler.go), the
// collectors runner (collectors_bridge.go), the docker auto-collector
// (docker_auto_collector.go), and tests.
func (s *apiServer) processHeartbeatRequest(req assets.HeartbeatRequest) (*assets.Asset, error) {
	return s.ensureResourcesDeps().ProcessHeartbeatRequest(req)
}
