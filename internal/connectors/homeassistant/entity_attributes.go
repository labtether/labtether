package homeassistant

import (
	"strings"

	"github.com/labtether/labtether/internal/securityruntime"
)

// Home Assistant publishes short-lived credentials as ordinary entity data.
// Camera and image entities carry an access_token, their entity_picture is a
// proxy URL with that token in its query string, and signed paths carry an
// authSig. Integrations also expose Wi-Fi keys, passwords and URLs with a user
// and password in them. Asset metadata is readable by every viewer, so none of
// this is copied there.
var hiddenAttributeKeyMarkers = []string{
	"token", "password", "passwd", "passphrase", "secret", "api_key", "apikey",
	"access_key", "private_key", "psk", "credential", "authsig",
}

var hiddenValueMarkers = []string{
	"token=", "access_token", "authsig=", "api_key=", "apikey=", "password=", "secret=",
}

func setEntityAttribute(metadata map[string]string, key string, value any) {
	if attributeKeyIsCredential(key) || valueCarriesCredential(value) {
		return
	}
	setMetadataValue(metadata, key, value)
}

// entityState returns the state to store for an entity. input_text and text
// entities in password mode hold the secret itself as their state, so only
// Home Assistant's own unavailable and unknown markers are kept for them.
func entityState(rawState any, attributes map[string]any) string {
	state := strings.TrimSpace(anyToString(rawState))
	switch strings.ToLower(state) {
	case "unavailable", "unknown":
		return state
	}
	if strings.EqualFold(strings.TrimSpace(anyToString(attributes["mode"])), "password") || valueCarriesCredential(state) {
		return ""
	}
	return state
}

func attributeKeyIsCredential(key string) bool {
	lowerKey := strings.ToLower(key)
	for _, marker := range hiddenAttributeKeyMarkers {
		if strings.Contains(lowerKey, marker) {
			return true
		}
	}
	return false
}

// valueCarriesCredential looks through nested maps and lists as well, because
// Home Assistant attributes are arbitrary JSON.
func valueCarriesCredential(value any) bool {
	switch typed := value.(type) {
	case string:
		lower := strings.ToLower(typed)
		for _, marker := range hiddenValueMarkers {
			if strings.Contains(lower, marker) {
				return true
			}
		}
		_, hasUserinfo := securityruntime.RedactURLUserinfoInText(typed)
		return hasUserinfo
	case map[string]any:
		for key, child := range typed {
			if attributeKeyIsCredential(key) || valueCarriesCredential(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if valueCarriesCredential(child) {
				return true
			}
		}
	}
	return false
}
