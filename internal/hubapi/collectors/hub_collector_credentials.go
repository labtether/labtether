package collectors

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/servicehttp"
)

var (
	errCollectorInlineSecret = errors.New("collector config contains inline secret material")
	errCollectorCredentialID = errors.New("collector config credential_id must be a string")
)

// validateCollectorConfig rejects secret material in collector configuration.
// Collector secrets must live in the encrypted credential store and be
// referenced by credential_id instead of being persisted in JSON config.
func validateCollectorConfig(config map[string]any) error {
	normalized, err := normalizeCollectorConfig(config)
	if err != nil {
		return err
	}
	if collectorConfigContainsSecret(normalized) {
		return errCollectorInlineSecret
	}
	return nil
}

func normalizeCollectorConfig(config map[string]any) (map[string]any, error) {
	if config == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func collectorConfigContainsSecret(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isCollectorSecretKey(key) || collectorConfigContainsSecret(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if collectorConfigContainsSecret(child) {
				return true
			}
		}
	}
	return false
}

func isCollectorSecretKey(key string) bool {
	canonical := strings.ToLower(strings.TrimSpace(key))
	// These canonical fields contain identifiers or header names, not secret
	// values. Variants stay subject to the denylist because the runtime only
	// consumes the exact canonical spelling.
	switch canonical {
	case "token_id", "api_key_header":
		return false
	}

	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)

	for _, marker := range []string{
		"password",
		"passwd",
		"passphrase",
		"privatekey",
		"secret",
		"apikey",
		"token",
		"authorization",
		"cookie",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "pass" || normalized == "pw" || normalized == "pwd" || strings.HasSuffix(normalized, "pwd")
}

func collectorCredentialID(config map[string]any) (string, error) {
	if config == nil {
		return "", nil
	}
	value, present := config["credential_id"]
	if !present {
		return "", nil
	}
	credentialID, ok := value.(string)
	if !ok {
		return "", errCollectorCredentialID
	}
	return strings.TrimSpace(credentialID), nil
}

func collectorUpdateInvokesExistingCredential(collector hubcollector.Collector, req hubcollector.UpdateCollectorRequest) bool {
	if req.Config != nil {
		return false
	}
	if req.Enabled != nil {
		// Disabling must remain available as a credential-independent kill
		// switch. Enabling authorizes the next scheduled invocation.
		return *req.Enabled
	}
	// Moving the schedule of an enabled collector can cause the bound
	// credential to be used sooner or more frequently.
	return collector.Enabled && req.IntervalSeconds != nil
}

func (d *Deps) authorizeCollectorConfig(w http.ResponseWriter, r *http.Request, config map[string]any) bool {
	if err := validateCollectorConfig(config); err != nil {
		if errors.Is(err, errCollectorInlineSecret) {
			servicehttp.WriteError(w, http.StatusBadRequest, "inline secrets are not allowed in collector config; use credential_id")
		} else {
			servicehttp.WriteError(w, http.StatusBadRequest, "collector config must contain valid JSON values")
		}
		return false
	}
	credentialID, err := collectorCredentialID(config)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "credential_id must be a string")
		return false
	}
	if credentialID == "" {
		return true
	}
	_, ok := d.loadAuthorizedCredentialProfile(w, r, credentialID)
	return ok
}

// loadAuthorizedCredentialProfile keeps existence checks behind the same
// credentials:use boundary as actual secret use. Callers must not reveal
// profile existence before this helper succeeds.
func (d *Deps) loadAuthorizedCredentialProfile(w http.ResponseWriter, r *http.Request, credentialID string) (credentials.Profile, bool) {
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return credentials.Profile{}, true
	}
	if !apiv2.RequireScope(w, r, "credentials:use") {
		return credentials.Profile{}, false
	}
	if d.CredentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return credentials.Profile{}, false
	}
	profile, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load credential profile")
		return credentials.Profile{}, false
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusBadRequest, "credential_id not found")
		return credentials.Profile{}, false
	}
	return profile, true
}

func redactHubCollector(collector hubcollector.Collector) hubcollector.Collector {
	collector.Config = redactCollectorConfig(collector.Config)
	return collector
}

func redactHubCollectors(collectors []hubcollector.Collector) []hubcollector.Collector {
	if collectors == nil {
		return nil
	}
	redacted := make([]hubcollector.Collector, len(collectors))
	for i, collector := range collectors {
		redacted[i] = redactHubCollector(collector)
	}
	return redacted
}

func redactCollectorConfig(config map[string]any) map[string]any {
	if config == nil {
		return nil
	}
	normalized, err := normalizeCollectorConfig(config)
	if err != nil {
		// A non-JSON value should never reach persistence. Fail closed if it
		// does rather than serializing a potentially sensitive custom value.
		return map[string]any{}
	}
	return redactCollectorConfigValue(normalized).(map[string]any)
}

func redactCollectorConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, child := range typed {
			if isCollectorSecretKey(key) {
				redacted[key] = RedactedConnectorSecret
				continue
			}
			redacted[key] = redactCollectorConfigValue(child)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for i, child := range typed {
			redacted[i] = redactCollectorConfigValue(child)
		}
		return redacted
	default:
		return value
	}
}
