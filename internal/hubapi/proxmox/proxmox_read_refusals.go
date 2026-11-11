package proxmox

import (
	"errors"
	"net/http"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/servicehttp"
)

// proxmoxReadRefused reports whether Proxmox answered but refused this read
// for the saved login (its "permission check failed").
func proxmoxReadRefused(err error) bool {
	var apiErr *proxmox.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden
}

// proxmoxReadWarning words a part of a device page that Proxmox did not give.
// A refusal says so; other failures keep the part's name and the error.
func proxmoxReadWarning(part string, err error) string {
	if proxmoxReadRefused(err) {
		return "Proxmox refused the " + part + " for the saved login."
	}
	return part + " unavailable: " + err.Error()
}

// proxmoxConfigPart names the main read of a device page by device kind.
func proxmoxConfigPart(kind string) string {
	switch kind {
	case "node":
		return "node status"
	case "qemu":
		return "VM settings"
	case "lxc":
		return "container settings"
	default:
		return "storage status"
	}
}

// writeProxmoxCephError answers a Ceph read Proxmox could not give.
func writeProxmoxCephError(w http.ResponseWriter, what string, err error) {
	switch {
	case proxmox.CephNotSetUp(err):
		servicehttp.WriteError(w, http.StatusNotFound, "Ceph is not set up on this Proxmox cluster.")
	case errors.Is(err, proxmox.ErrNoReadableNode):
		servicehttp.WriteError(w, http.StatusConflict,
			"The saved login cannot see an online Proxmox node, so Ceph cannot be read. Give that login access to a node on Proxmox.")
	default:
		writeProxmoxUpstreamError(w, what, err)
	}
}
