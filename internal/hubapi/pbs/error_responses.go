package pbs

import (
	"net/http"

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
