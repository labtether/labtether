package pbs

import (
	"net/http"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

// writePBSError records the full failure for operators while returning only a
// stable, non-sensitive description to API clients.
func writePBSError(w http.ResponseWriter, status int, clientMessage string, err error) {
	securityruntime.Logf("pbs: %s: %v", clientMessage, err)
	servicehttp.WriteError(w, status, clientMessage)
}

// pbsWarning is the warning equivalent of writePBSError for successful or
// stale-cache responses that include a warnings array.
func pbsWarning(clientMessage string, err error) string {
	securityruntime.Logf("pbs: %s: %v", clientMessage, err)
	return clientMessage
}

// writePBSRuntimeError answers a request whose Proxmox Backup Server
// connection could not be loaded. No connection, or a turned-off one, is a
// 404 in words; anything else stays a 502. forDevice says the request named
// one device.
func writePBSRuntimeError(w http.ResponseWriter, err error, forDevice bool) {
	if shared.WriteCollectorSetupError(w, err, "Proxmox Backup Server", forDevice) {
		return
	}
	writePBSError(w, http.StatusBadGateway, "pbs runtime unavailable", err)
}
