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
