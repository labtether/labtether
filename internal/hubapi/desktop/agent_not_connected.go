package desktop

import (
	"net/http"

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
