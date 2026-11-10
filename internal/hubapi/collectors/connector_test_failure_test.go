package collectors

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A failed Test connection answered 502, so the Add device dialog could only
// say "An internal error occurred." The answer must name a reason the person
// can act on, and must never carry the address, the secret or the other
// system's own text.
func TestConnectorTestFailureGivesAReasonAndNoInternals(t *testing.T) {
	// A made-up value that stands for the login the person typed.
	const planted = "planted-login-value"
	cases := []struct{ failure, want string }{
		{`truenas ws auth: server rejected api key`,
			"Proxmox refused the login. Check the user or token name and the secret."},
		{`truenas ws dial wss://192.0.2.10/api/current: websocket: bad handshake`,
			"That address did not answer like Proxmox. Check the address, port and path."},
		{`Get "https://192.0.2.10:8006/x": dial tcp 192.0.2.10:8006: connect: connection timed out`,
			"Could not reach Proxmox at that address. Check the address and port."},
		{`Get "https://192.0.2.10:8006/api2/json/version": dial tcp 192.0.2.10:8006: connect: connection refused`,
			"Could not reach Proxmox at that address. Check the address and port."},
		{`Get "https://pve.example.test:8006/x": dial tcp: lookup pve.example.test: no such host`,
			"Could not reach Proxmox at that address. Check the address and port."},
		{`Get "https://192.0.2.10:8006/x": tls: failed to verify certificate: x509: certificate signed by unknown authority`,
			"The certificate of Proxmox is not trusted. Add its CA certificate or turn on Skip TLS verification."},
		{`Get "https://192.0.2.10:8006/x": http: server gave HTTP response to HTTPS client`,
			"That address does not answer with HTTPS. Check the address and port of Proxmox."},
		{`proxmox api returned 401: authentication failure token_secret=` + planted,
			"Proxmox refused the login. Check the user or token name and the secret."},
		{`proxmox api returned 404: <html><body>nginx</body></html>`,
			"That address did not answer like Proxmox. Check the address, port and path."},
		{`outbound private host "192.0.2.10" is not allowed`,
			"The Hub is set not to connect to private, loopback or link-local addresses like that one."},
		{`proxmox api returned 500: {"error":"token_secret=` + planted + `"}`,
			"Proxmox answered with an error, so the test did not finish."},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		writeConnectorTestFailure(rec, "Proxmox", tc.failure, planted)
		body := rec.Body.String()
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, tc.want) {
			t.Fatalf("%q: got %d %s, want 422 %q", tc.failure, rec.Code, body, tc.want)
		}
		for _, internal := range []string{planted, "192.0.2.10", "pve.example.test", "dial tcp", "nginx", "x509"} {
			if strings.Contains(body, internal) {
				t.Fatalf("%q: answer carries %q: %s", tc.failure, internal, body)
			}
		}
	}
}
