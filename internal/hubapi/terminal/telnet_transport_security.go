package terminal

import (
	"errors"
	"net/http"

	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

func validateInsecureTelnetTransport(allowInsecure bool) error {
	if !allowInsecure {
		return errors.New("plain Telnet requires allow_insecure_telnet=true")
	}
	if !securityruntime.InsecureTransportAllowed() {
		return errors.New("plain Telnet requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
	}
	return nil
}

func requireInsecureTelnetTransport(w http.ResponseWriter, allowInsecure bool) bool {
	if err := validateInsecureTelnetTransport(allowInsecure); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}
