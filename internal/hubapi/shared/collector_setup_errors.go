package shared

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/labtether/labtether/internal/servicehttp"
)

// WriteCollectorSetupError answers a connector request whose connection is
// missing, turned off or of another kind. That is the user's setup, not an
// upstream failure, so it gets a 4xx in words: clients replace every 5xx
// message with "An internal error occurred." product is the name users know,
// such as "Proxmox". forDevice says the request named one device. It reports
// false, writing nothing, for any other error.
func WriteCollectorSetupError(w http.ResponseWriter, err error, product string, forDevice bool) bool {
	switch {
	case errors.Is(err, ErrCollectorWrongType):
		servicehttp.WriteError(w, http.StatusConflict,
			fmt.Sprintf("This device is linked to a connection that is not a %s connection.", product))
	case errors.Is(err, ErrCollectorNotActive) && forDevice:
		servicehttp.WriteError(w, http.StatusNotFound,
			fmt.Sprintf("This device's %s connection was removed or turned off.", product))
	case errors.Is(err, ErrCollectorNotActive):
		servicehttp.WriteError(w, http.StatusNotFound,
			fmt.Sprintf("%s is not set up. Connect it from Devices → Add device.", product))
	default:
		return false
	}
	return true
}
