package resources

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// writeAgentNotConnected answers a device request when the device's agent is
// not connected. It is a 409 with the same words exec uses, because clients
// show a 4xx message as is but replace every 5xx message with a generic
// "An internal error occurred." An ID that names no device is a 404.
func (d *Deps) writeAgentNotConnected(w http.ResponseWriter, assetID string) {
	if d.AssetStore != nil {
		asset, ok, err := d.AssetStore.GetAsset(assetID)
		if err == nil && !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "device not found")
			return
		}
		if err == nil && ok && !asset.LastSeenAt.IsZero() {
			servicehttp.WriteError(w, http.StatusConflict, fmt.Sprintf("agent is not connected (last seen: %s)",
				asset.LastSeenAt.UTC().Format(time.RFC3339)))
			return
		}
	}
	servicehttp.WriteError(w, http.StatusConflict, "agent is not connected")
}
