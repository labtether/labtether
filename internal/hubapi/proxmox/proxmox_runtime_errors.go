package proxmox

import (
	"net/http"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// writeProxmoxRuntimeError answers a request whose Proxmox connection could
// not be loaded. No connection, or a turned-off one, is a 404 in words;
// anything else stays a 502. forDevice says the request named one device.
func writeProxmoxRuntimeError(w http.ResponseWriter, err error, forDevice bool) {
	if shared.WriteCollectorSetupError(w, err, "Proxmox", forDevice) {
		return
	}
	servicehttp.WriteError(w, http.StatusBadGateway, "proxmox runtime unavailable: "+shared.SanitizeUpstreamError(err.Error()))
}

// writeNotProxmoxDevice answers a Proxmox request for an ID that is not a
// Proxmox device: "device not found" when the ID names no device at all.
func (d *Deps) writeNotProxmoxDevice(w http.ResponseWriter, assetID string) {
	if d.AssetStore != nil {
		if _, ok, err := d.AssetStore.GetAsset(assetID); err == nil && !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "device not found")
			return
		}
	}
	servicehttp.WriteError(w, http.StatusNotFound, "this is not a Proxmox device")
}

// writeProxmoxUpstreamError answers a Proxmox request that failed after its
// connection was loaded. A Proxmox that cannot be reached, or refuses the
// saved login, is said in fixed words; anything else stays a hidden 502 with
// what in the log.
func writeProxmoxUpstreamError(w http.ResponseWriter, what string, err error) {
	if shared.WriteUpstreamUnreachable(w, err, "Proxmox") {
		return
	}
	servicehttp.WriteError(w, http.StatusBadGateway, what+shared.SanitizeUpstreamError(err.Error()))
}
