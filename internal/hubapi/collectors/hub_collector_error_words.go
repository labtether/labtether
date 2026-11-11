package collectors

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/labtether/labtether/internal/securityruntime"
)

// A collection that failed kept Go's error as last_error ("portainer discovery
// failed: portainer endpoints: Get \"https://…\": dial tcp …: connection
// refused"), and Add device showed it in its "sync failed" line. last_error
// now holds a sentence; the raw error goes to the Hub log.

var collectorProductNames = map[string]string{
	"proxmox": "Proxmox", "pbs": "Proxmox Backup Server", "truenas": "TrueNAS",
	"portainer": "Portainer", "homeassistant": "Home Assistant", "docker": "Docker",
}

// rawCollectorError marks Go and HTTP error text, which carries addresses,
// URLs and upstream replies that people cannot act on.
var rawCollectorError = regexp.MustCompile(`(?i)dial tcp|no such host|connection refused|i/o timeout|deadline exceeded|` +
	`x509:|tls:|\b(get|post|put|delete) "https?://|wss?://|\bapi returned \d{3}\b|insecure url scheme|eof\b|bad handshake`)

// collectorErrorWords returns the sentence for a failed collection, and false
// when the message is already one (or the collector type has no product name).
func collectorErrorWords(collectorType, message string) (string, bool) {
	product, ok := collectorProductNames[strings.ToLower(strings.TrimSpace(collectorType))]
	if !ok || !rawCollectorError.MatchString(message) {
		return message, false
	}
	text := strings.ToLower(message)
	has := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(text, part) {
				return true
			}
		}
		return false
	}
	switch {
	case has("insecure url scheme"):
		return fmt.Sprintf("The Hub connects to %s over https only. Use an https:// address, or let the Hub use http by setting LABTETHER_ALLOW_INSECURE_TRANSPORT=true.", product), true
	case has("x509:", "tls:"):
		return fmt.Sprintf("The certificate of %s is not trusted or does not match its address. Check the connection's certificate settings.", product), true
	case has("returned 401"):
		return fmt.Sprintf("%s refused the login. Check the user or token name and the secret.", product), true
	case has("returned 403"):
		return fmt.Sprintf("%s refused part of the collection for the saved login. Give that login read access on %s.", product, product), true
	case has("dial tcp", "no such host", "connection refused", "i/o timeout", "deadline exceeded", "eof", "bad handshake"):
		return fmt.Sprintf("Could not reach %s. Check the connection's address and port.", product), true
	}
	return fmt.Sprintf("%s answered with an error during collection. The Hub log has the details.", product), true
}

// collectorStatusMessage is the last_error to store for a collector run.
func (d *Deps) collectorStatusMessage(collectorID, status, message string) string {
	if strings.EqualFold(status, "ok") || strings.TrimSpace(message) == "" || d.HubCollectorStore == nil {
		return message
	}
	collector, ok, err := d.HubCollectorStore.GetHubCollector(collectorID)
	if err != nil || !ok {
		return message
	}
	words, changed := collectorErrorWords(collector.CollectorType, message)
	if changed {
		securityruntime.Logf("hub collector %s (%s): %s", collectorID, collector.CollectorType, SanitizeConnectorErrorMessage(message))
	}
	return words
}
