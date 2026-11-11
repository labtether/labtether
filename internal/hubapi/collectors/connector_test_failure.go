package collectors

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

var connectorLoginRefusedPattern = regexp.MustCompile(`\b(401|403)\b|authentication fail|unauthori[sz]ed|forbidden|permission denied|access denied|invalid (api )?(key|token|credentials|username)|rejected (the )?(api )?(key|token|login)`)

// connectorTestReason picks the words for a failed Test connection. The
// failure text only selects the reason and is never sent: it can carry the
// address, a token, or whatever the other system answered.
func connectorTestReason(product, failure string) string {
	text := strings.ToLower(failure)
	has := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(text, part) {
				return true
			}
		}
		return false
	}
	switch {
	case has("no endpoints are visible"):
		return fmt.Sprintf("%s answered, but this login sees no environments.", product)
	case has("allowed only to an address on your own network", "outside your network"):
		return fmt.Sprintf("Plain http to %s works only for an address on your own network: a private or tailnet address, or a .local or .ts.net name. Use such an address, or https.", product)
	case has("insecure url scheme"):
		return fmt.Sprintf("The Hub connects to %s over https only. Use an https:// address, or let the Hub use http by setting LABTETHER_ALLOW_INSECURE_TRANSPORT=true.", product)
	case has("outbound") && has("not allowlisted"):
		return "The Hub's list of allowed outbound addresses does not include that one. Add it to LABTETHER_OUTBOUND_ALLOWED_HOSTS."
	case has("outbound") && has("is not allowed"):
		return "The Hub is set not to connect to private, loopback or link-local addresses like that one."
	case has("http response to https client", "does not look like a tls handshake"):
		return fmt.Sprintf("That address does not answer with HTTPS. Check the address and port of %s.", product)
	// Go checks a certificate's dates first, then the name, then the issuer,
	// and stops at the first problem. Each needs a different fix: renew it,
	// fix the clock, use the right address, or add the CA.
	case has("certificate has expired or is not yet valid") && has(" is before "):
		return fmt.Sprintf("The certificate of %s is not valid yet. Check the date and time on %s and on the Hub.", product, product)
	case has("certificate has expired or is not yet valid"):
		return fmt.Sprintf("The certificate of %s has expired. Renew it on %s.", product, product)
	case has("doesn't contain any ip sans", "certificate is valid for", "certificate is not valid for any names"):
		return fmt.Sprintf("The certificate of %s does not match that address. Use the name or IP address the certificate was made for.", product)
	case has("x509:", "tls:", "certificate"):
		return fmt.Sprintf("The certificate of %s is not trusted. Add its CA certificate or turn on Skip TLS verification.", product)
	case has("connection refused", "no such host", "no route to host", "network is unreachable", "host is down", "timeout", "timed out", "deadline exceeded", "connection reset", "eof"):
		return fmt.Sprintf("Could not reach %s at that address. Check the address and port.", product)
	case connectorLoginRefusedPattern.MatchString(text):
		return fmt.Sprintf("%s refused the login. Check the user or token name and the secret.", product)
	case has(" 404", " 405", " 501", "not found", "invalid character", "cannot unmarshal", "unexpected", "did not return", "bad handshake", "malformed", "mismatch"):
		return fmt.Sprintf("That address did not answer like %s. Check the address, port and path.", product)
	}
	return fmt.Sprintf("%s answered with an error, so the test did not finish.", product)
}

// writeConnectorTestFailure answers a failed Test connection with a reason
// the person can act on. It is a 422, not a 5xx: clients replace every 5xx
// text with "An internal error occurred.", so the Add device dialog could
// never say why a test failed. The full failure goes to the Hub log with the
// given secrets removed.
func writeConnectorTestFailure(w http.ResponseWriter, product, failure string, secrets ...string) {
	securityruntime.Logf("connector test: %s: %s", product, SanitizeConnectorErrorMessage(failure, secrets...))
	servicehttp.WriteError(w, http.StatusUnprocessableEntity, connectorTestReason(product, failure))
}
