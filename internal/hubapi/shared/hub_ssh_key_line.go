package shared

import "strings"

// HubAuthorizedKeyComment ends every authorized_keys line the Hub asks an
// agent to add. Uninstall removes lines with this comment, which covers keys
// from before a Hub key rotation and never matches other keys.
const HubAuthorizedKeyComment = "labtether-hub"

// HubAuthorizedKeyLine returns the authorized_keys line the Hub installs for
// its public key: the key type and data followed by HubAuthorizedKeyComment.
func HubAuthorizedKeyLine(publicKey string) string {
	fields := strings.Fields(publicKey)
	if len(fields) < 2 {
		return strings.TrimSpace(publicKey)
	}
	return fields[0] + " " + fields[1] + " " + HubAuthorizedKeyComment
}

// HubAuthorizedKeyRemovals returns every line to remove for a Hub public key:
// the plain line Hubs before this comment installed, then the commented line.
func HubAuthorizedKeyRemovals(publicKey string) []string {
	plain := strings.TrimSpace(publicKey)
	marked := HubAuthorizedKeyLine(publicKey)
	if plain == marked {
		return []string{plain}
	}
	return []string{plain, marked}
}
