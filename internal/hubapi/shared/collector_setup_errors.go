package shared

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"

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
	case errors.Is(err, ErrCollectorLoginMissing):
		servicehttp.WriteError(w, http.StatusConflict,
			fmt.Sprintf("The saved login for the %s connection is missing. Enter it again in Settings.", product))
	case errors.Is(err, ErrCollectorIncomplete):
		servicehttp.WriteError(w, http.StatusConflict,
			fmt.Sprintf("The %s connection has no address or login. Edit it in Settings.", product))
	default:
		return false
	}
	return true
}

// A connect-phase failure: nothing reached the system, so no request ran.
var upstreamUnreachablePattern = regexp.MustCompile(`(?i)connection refused|no such host|no route to host|network is unreachable|` +
	`dial tcp[^:]*:[^:]*: i/o timeout|while waiting for connection|x509: |tls: `)

// The system answered but refused the saved login.
var upstreamLoginRefusedPattern = regexp.MustCompile(`(?i)\b(returned|HTTP) (401|403)\b`)

// UpstreamUnreachable reports whether a connector request failed because the
// system could not be reached (DNS, connect, TLS) or refused the saved login.
// A timeout after the request was sent is not counted: a change may have run.
func UpstreamUnreachable(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	var opErr *net.OpError
	if errors.As(err, &dnsErr) || (errors.As(err, &opErr) && opErr.Op == "dial") {
		return true
	}
	text := err.Error()
	return upstreamUnreachablePattern.MatchString(text) || upstreamLoginRefusedPattern.MatchString(text)
}

// WriteUpstreamUnreachable answers a connector request whose system could not
// be reached or refused its login. The Hub hides every 5xx text, so this is a
// 409 with fixed words that name no address, like "agent is not connected"
// for a device. It reports false, writing nothing, for any other error.
func WriteUpstreamUnreachable(w http.ResponseWriter, err error, product string) bool {
	if !UpstreamUnreachable(err) {
		return false
	}
	servicehttp.WriteError(w, http.StatusConflict, UnreachableMessage(product))
	return true
}

// UnreachableMessage is the fixed sentence for a system that could not be
// reached or refused its login. It names no address.
func UnreachableMessage(product string) string {
	return fmt.Sprintf("Could not reach %s. Check the connection's address and login in Settings.", product)
}
