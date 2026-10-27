package homeassistant

import "strings"

// Home Assistant publishes short-lived credentials as ordinary entity
// attributes. Camera and image entities carry an access_token, and their
// entity_picture is a proxy URL with that token in its query string. Anyone
// holding either can pull the camera feed without signing in to Home
// Assistant, so such attributes never become asset metadata, which every
// viewer can read.
var hiddenAttributeMarkers = []string{"token", "password", "passwd", "secret", "api_key", "apikey"}

func setEntityAttribute(metadata map[string]string, key string, value any) {
	if entityAttributeCarriesCredential(key, value) {
		return
	}
	setMetadataValue(metadata, key, value)
}

func entityAttributeCarriesCredential(key string, value any) bool {
	lowerKey := strings.ToLower(key)
	for _, marker := range hiddenAttributeMarkers {
		if strings.Contains(lowerKey, marker) {
			return true
		}
	}
	serialized := strings.ToLower(stringifyMetadataValue(value))
	return strings.Contains(serialized, "token=") || strings.Contains(serialized, "access_token")
}
