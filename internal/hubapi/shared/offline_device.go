package shared

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/persistence"
)

// OfflineDeviceAnswer gives the status and words for a device request whose
// agent is not connected. Clients show a 4xx message as it is but replace
// every 5xx message with "An internal error occurred.", so the answer is a
// 404 "device not found" for an ID that names no device, and otherwise a 409
// "agent is not connected" with the time the device was last seen.
func OfflineDeviceAnswer(store persistence.AssetStore, assetID string) (int, string) {
	if store != nil {
		asset, ok, err := store.GetAsset(assetID)
		if err == nil && !ok {
			return http.StatusNotFound, "device not found"
		}
		if err == nil && !asset.LastSeenAt.IsZero() {
			return http.StatusConflict, fmt.Sprintf("agent is not connected (last seen: %s)",
				asset.LastSeenAt.UTC().Format(time.RFC3339))
		}
	}
	return http.StatusConflict, "agent is not connected"
}
