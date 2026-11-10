package resources

import (
	"net/http"

	"github.com/labtether/labtether/internal/servicehttp"
)

// writeIfDeviceMissing answers 404 "device not found" when a request names
// one device by an ID that names none, and reports true. The Map routes
// answered such a request 200 with an empty list, which reads as "this
// device has no links". A store error is not treated as missing.
func (d *Deps) writeIfDeviceMissing(w http.ResponseWriter, assetID string) bool {
	if d.AssetStore == nil {
		return false
	}
	if _, ok, err := d.AssetStore.GetAsset(assetID); err != nil || ok {
		return false
	}
	servicehttp.WriteError(w, http.StatusNotFound, "device not found")
	return true
}
