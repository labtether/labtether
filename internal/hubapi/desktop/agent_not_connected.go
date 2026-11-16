package desktop

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// writeAgentNotConnected answers a Remote View request when the device's
// agent is not connected: 409 "agent is not connected (last seen: ...)", or
// 404 "device not found" for an ID that names no device. A 502 here reached
// users as "An internal error occurred." See shared.OfflineDeviceAnswer.
func (d *Deps) writeAgentNotConnected(w http.ResponseWriter, assetID string) {
	status, message := shared.OfflineDeviceAnswer(d.AssetStore, assetID)
	servicehttp.WriteError(w, status, message)
}

// writeNoDesktopRoute answers a stream request that no route can serve. A
// device that is reached through its agent has no route only while that
// agent is away, so it gets the agent answer; this used to be a 502 "direct
// VNC proxy is disabled", shown as "An internal error occurred."
func (d *Deps) writeNoDesktopRoute(w http.ResponseWriter, assetID string) {
	if d.AssetStore != nil {
		asset, ok, err := d.AssetStore.GetAsset(assetID)
		if err == nil && ok && strings.EqualFold(strings.TrimSpace(asset.Source), "agent") {
			d.writeAgentNotConnected(w, assetID)
			return
		}
	}
	servicehttp.WriteError(w, http.StatusConflict, "Remote View has no way to reach this device.")
}

// refuseUnadvertised answers 422 and returns true when conn's agent said its
// advertisement is complete and left msgType out. Nothing is sent.
func refuseUnadvertised(w http.ResponseWriter, conn *agentmgr.AgentConn, msgType string) bool {
	if agentcaps.ConnOffers(conn, msgType) {
		return false
	}
	servicehttp.WriteAgentUnsupported(w)
	return true
}
