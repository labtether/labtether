package resources

import (
	"net/http"

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
