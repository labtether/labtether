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

// Mutation failures cannot prove the remote side had no effect. Keep details
// in the redacted server log while giving callers a safe, explicit outcome.
func writeTrueNASMutationError(w http.ResponseWriter, status int, message string, err error) {
	securityruntime.Logf("truenas: %s: %v", message, err)
	// WriteError masks every 5xx message. This fixed text is deliberately public
	// and contains no provider error, credential, path or request payload.
	servicehttp.WriteJSON(w, status, map[string]any{
		"error":   "TrueNAS request failed; request was not retried and outcome may be unknown. Check device state before trying again.",
		"outcome": "unknown", "retried": false,
	})
}
