package shared

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	RuntimeSettingKeyCannotBeEmptyError       = "runtime setting key cannot be empty"
	RuntimeSettingsResetKeysRequiredError     = "runtime settings reset keys are required"
	RuntimeSettingsLoadFailureError           = "failed to load runtime settings"
	RuntimeSettingsResetUnknownKeyErrorFormat = "unknown runtime setting key: %s"
)

// NormalizeRuntimeOverrides validates and normalises a map of raw key→value
// overrides. Returns an empty map (not an error) for empty input.
func NormalizeRuntimeOverrides(values map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return map[string]string{}, nil
	}

	out := make(map[string]string, len(values))
	for rawKey, rawValue := range values {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return nil, errors.New(RuntimeSettingKeyCannotBeEmptyError)
		}
		definition, ok := runtimesettings.DefinitionByKey(key)
		if !ok {
			return nil, fmt.Errorf(RuntimeSettingsResetUnknownKeyErrorFormat, key)
		}
		normalized, err := runtimesettings.NormalizeValue(definition, rawValue)
		if err != nil {
			return nil, err
		}
		out[key] = normalized
	}
	return out, nil
}

// SanitizeRuntimeSettingKeys validates and deduplicates a list of runtime
// setting keys. Returns nil (not an error) for empty input.
func SanitizeRuntimeSettingKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := runtimesettings.DefinitionByKey(key); !ok {
			return nil, fmt.Errorf(RuntimeSettingsResetUnknownKeyErrorFormat, key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil, errors.New(RuntimeSettingsResetKeysRequiredError)
	}
	return out, nil
}

// WriteRuntimeSettingsPayload loads all runtime setting overrides from the
// store and writes the full settings payload (definitions + overrides) as JSON.
func WriteRuntimeSettingsPayload(w http.ResponseWriter, store persistence.RuntimeSettingsStore) {
	overrides, err := store.ListRuntimeSettingOverrides()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, RuntimeSettingsLoadFailureError)
		return
	}

	payload := RuntimeSettingsPayload{
		Settings:  make([]RuntimeSettingEntry, 0, len(runtimesettings.Definitions())),
		Overrides: make(map[string]string, len(overrides)),
	}
	for _, definition := range runtimesettings.Definitions() {
		envValue := runtimesettings.ResolveEnvValue(definition, os.Getenv)

		overrideValue := ""
		if rawOverride, ok := overrides[definition.Key]; ok {
			normalizedOverride, normErr := runtimesettings.NormalizeValue(definition, rawOverride)
			if normErr == nil {
				overrideValue = normalizedOverride
				payload.Overrides[definition.Key] = normalizedOverride
			}
		}

		effectiveValue, source := runtimesettings.EffectiveValue(definition, envValue, overrideValue)
		entry := RuntimeSettingEntry{
			Key:            definition.Key,
			Label:          definition.Label,
			Description:    definition.Description,
			Scope:          definition.Scope,
			Type:           string(definition.Type),
			EnvVar:         definition.EnvVar,
			DefaultValue:   definition.DefaultValue,
			EnvValue:       envValue,
			OverrideValue:  overrideValue,
			EffectiveValue: effectiveValue,
			Source:         string(source),
			MinInt:         definition.MinInt,
			MaxInt:         definition.MaxInt,
		}
		if len(definition.AllowedValues) > 0 {
			entry.AllowedValues = append([]string(nil), definition.AllowedValues...)
		}
		payload.Settings = append(payload.Settings, entry)
	}

	servicehttp.WriteJSON(w, http.StatusOK, payload)
}
