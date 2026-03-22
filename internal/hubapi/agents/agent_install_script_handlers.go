package agents

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// handleAgentInstallScript serves a self-contained bash install script for
// the LabTether agent. The endpoint is intentionally public (no auth) so it
// can be piped directly into bash: curl <hub>/install.sh | sudo bash
func (d *Deps) HandleAgentInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hubURL := d.ResolveHubURL(r)
	wsURL := strings.TrimRight(shared.HTTPURLToWS(hubURL), "/") + "/ws/agent"

	script := GenerateInstallScript(hubURL, wsURL)
	w.Header().Set("Content-Type", "text/x-shellscript")
	w.WriteHeader(http.StatusOK)
	// #nosec G705 -- script is plain text shell output, not HTML rendering.
	_, _ = io.WriteString(w, script)
}

// handleAgentBootstrapScript serves a self-contained bootstrap script for
// self-signed TLS environments. The script performs CA bootstrap + fingerprint
// verification, then downloads and executes install.sh using verified TLS.
//
// Query parameters:
//
//	ca_fingerprint_sha256 — required; expected lowercase/uppercase SHA-256 hex of CA cert
//
// Example:
//
//	curl -kfsSL "https://hub:8443/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=<hex>" | sudo bash -s -- --enrollment-token <token>
func (d *Deps) HandleAgentBootstrapScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	expectedFingerprint, ok := SanitizeSHA256Hex(r.URL.Query().Get("ca_fingerprint_sha256"))
	if !ok {
		http.Error(w, "ca_fingerprint_sha256 query parameter is required and must be 64 hex characters", http.StatusBadRequest)
		return
	}

	hubURL := d.ResolveHubURL(r)
	script := GenerateBootstrapScript(hubURL, expectedFingerprint)

	w.Header().Set("Content-Type", "text/x-shellscript")
	w.WriteHeader(http.StatusOK)
	// #nosec G705 -- script is plain text shell output, not HTML rendering.
	_, _ = io.WriteString(w, script)
}

func SanitizeSHA256Hex(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if len(value) != 64 {
		return "", false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return "", false
	}
	return value, true
}

// GenerateBootstrapScript returns a self-contained bootstrap script that:
//  1. downloads the hub CA cert with bootstrap transport (-k/no-check),
//  2. verifies the downloaded cert against the expected SHA-256 fingerprint,
//  3. installs the CA locally (+ optional system trust update),
//  4. fetches install.sh using CA-verified TLS and executes it with --tls-ca-file.
func GenerateBootstrapScript(hubURL, expectedCAFingerprint string) string {
	return fmt.Sprintf(agentBootstrapScriptTemplate(), hubURL, expectedCAFingerprint)
}

// GenerateInstallScript returns the full text of the bash install script.
// hubURL is the HTTP base URL of the hub (e.g. "http://192.168.1.10:8080").
// wsURL  is the WebSocket agent URL  (e.g. "ws://192.168.1.10:8080/ws/agent").
func GenerateInstallScript(hubURL, wsURL string) string {
	return fmt.Sprintf(agentInstallScriptTemplate(), hubURL, wsURL)
}
