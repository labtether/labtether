package truenas

import (
	"net/http"

	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

// writeTrueNASError records the full failure for operators while returning
// only a stable, non-sensitive description to API clients.
func writeTrueNASError(w http.ResponseWriter, status int, clientMessage string, err error) {
	securityruntime.Logf("truenas: %s: %v", clientMessage, err)
	servicehttp.WriteError(w, status, clientMessage)
}

// trueNASWarning is the warning equivalent of writeTrueNASError for successful
// or stale-cache responses that include a warnings array.
func trueNASWarning(clientMessage string, err error) string {
	securityruntime.Logf("truenas: %s: %v", clientMessage, err)
	return clientMessage
}
