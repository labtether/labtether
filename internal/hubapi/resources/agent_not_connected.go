package resources

import (
	"net/http"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// writeAgentNotConnected answers a device request when the device's agent is
// not connected: 409 with the same words exec uses, or 404 "device not found"
// for an ID that names no device. See shared.OfflineDeviceAnswer.
func (d *Deps) writeAgentNotConnected(w http.ResponseWriter, assetID string) {
	status, message := shared.OfflineDeviceAnswer(d.AssetStore, assetID)
	servicehttp.WriteError(w, status, message)
}

// refuseUnadvertised answers 422 and returns true when the device's agent
// is connected, said its advertisement is complete and left msgType out.
// Nothing is sent to the agent.
func (d *Deps) refuseUnadvertised(w http.ResponseWriter, assetID, msgType string) bool {
	if d.AgentMgr == nil {
		return false
	}
	conn, ok := d.AgentMgr.Get(assetID)
	return ok && refuseUnadvertisedConn(w, conn, msgType)
}

// refuseUnadvertisedConn is refuseUnadvertised for a connection in hand.
func refuseUnadvertisedConn(w http.ResponseWriter, conn *agentmgr.AgentConn, msgType string) bool {
	if agentcaps.ConnOffers(conn, msgType) {
		return false
	}
	servicehttp.WriteAgentUnsupported(w)
	return true
}
