package shared

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/secrets"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	RuntimeSettingKeyCannotBeEmptyError       = "runtime setting key cannot be empty"
	RuntimeSettingsResetKeysRequiredError     = "runtime settings reset keys are required"
	RuntimeSettingsLoadFailureError           = "failed to load runtime settings"
	RuntimeSettingsResetUnknownKeyErrorFormat = "unknown runtime setting key: %s"
	runtimeSettingSecretAADPrefix             = "labtether:runtime-setting:"
)

// PrepareRuntimeOverridesForStorage encrypts every sensitive override before
// it reaches persistence. Each ciphertext is bound to its setting key so it
// cannot be transplanted to another setting row.
func PrepareRuntimeOverridesForStorage(values map[string]string, manager *secrets.Manager) (map[string]string, error) {
	out := make(map[string]string, len(values))
	for key, value := range values {
		definition, ok := runtimesettings.DefinitionByKey(key)
		if !ok {
			return nil, fmt.Errorf(RuntimeSettingsResetUnknownKeyErrorFormat, key)
		}
		if !definition.Sensitive {
			out[key] = value
			continue
		}
		encrypted, err := manager.EncryptString(value, runtimeSettingSecretAAD(key))
		if err != nil {
			return nil, fmt.Errorf("encrypt runtime setting %s: %w", key, err)
		}
		out[key] = encrypted
	}
	return out, nil
}

// NonSensitiveRuntimeOverrides strips secret values before applying runtime
// settings to broad consumers that have no reason to handle credentials.
func NonSensitiveRuntimeOverrides(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		definition, ok := runtimesettings.DefinitionByKey(key)
		if ok && definition.Sensitive {
			continue
		}
		out[key] = value
	}
	return out
}

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

// ResolveRuntimeSettingEffectiveValues loads, validates, and resolves all
// settings. Sensitive legacy plaintext overrides are migrated in place to
// key-bound ciphertext before values are returned to the trusted caller.
// Callers must never serialize the returned map.
func ResolveRuntimeSettingEffectiveValues(store persistence.RuntimeSettingsStore, manager *secrets.Manager) (map[string]string, map[string]runtimesettings.Source, error) {
	overrides, err := loadRuntimeSettingOverrides(store, manager)
	if err != nil {
		return nil, nil, err
	}

	values := make(map[string]string, len(runtimesettings.Definitions()))
	sources := make(map[string]runtimesettings.Source, len(runtimesettings.Definitions()))
	for _, definition := range runtimesettings.Definitions() {
		envValue := runtimesettings.ResolveEnvValue(definition, os.Getenv)
		effectiveValue, source := runtimesettings.EffectiveValue(definition, envValue, overrides[definition.Key])
		values[definition.Key] = effectiveValue
		sources[definition.Key] = source
	}
	return values, sources, nil
}

// BuildRuntimeSettingsPayload builds the public API representation. Secret
// values are resolved only long enough to compute source/configured metadata;
// plaintext and ciphertext are omitted from every response field.
func BuildRuntimeSettingsPayload(store persistence.RuntimeSettingsStore, manager *secrets.Manager) (RuntimeSettingsPayload, error) {
	overrides, err := loadRuntimeSettingOverrides(store, manager)
	if err != nil {
		return RuntimeSettingsPayload{}, err
	}

	payload := RuntimeSettingsPayload{
		Settings:  make([]RuntimeSettingEntry, 0, len(runtimesettings.Definitions())),
		Overrides: make(map[string]string, len(overrides)),
	}
	for _, definition := range runtimesettings.Definitions() {
		envValue := runtimesettings.ResolveEnvValue(definition, os.Getenv)

		overrideValue := ""
		if normalizedOverride, ok := overrides[definition.Key]; ok {
			overrideValue = normalizedOverride
			if !definition.Sensitive {
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
			Sensitive:      definition.Sensitive,
			Configured:     definition.Sensitive && effectiveValue != "",
			MinInt:         definition.MinInt,
			MaxInt:         definition.MaxInt,
		}
		if len(definition.AllowedValues) > 0 {
			entry.AllowedValues = append([]string(nil), definition.AllowedValues...)
		}
		if definition.Sensitive {
			entry.DefaultValue = ""
			entry.EnvValue = ""
			entry.OverrideValue = ""
			entry.EffectiveValue = ""
		}
		payload.Settings = append(payload.Settings, entry)
	}
	return payload, nil
}

// WriteRuntimeSettingsPayload writes a redacted full settings payload.
func WriteRuntimeSettingsPayload(w http.ResponseWriter, store persistence.RuntimeSettingsStore, manager *secrets.Manager) error {
	payload, err := BuildRuntimeSettingsPayload(store, manager)
	if err != nil {
		return err
	}

	servicehttp.WriteJSON(w, http.StatusOK, payload)
	return nil
}

func loadRuntimeSettingOverrides(store persistence.RuntimeSettingsStore, manager *secrets.Manager) (map[string]string, error) {
	rawOverrides, err := store.ListRuntimeSettingOverrides()
	if err != nil {
		return nil, err
	}

	resolved := make(map[string]string, len(rawOverrides))
	migrations := make(map[string]string)
	for key, raw := range rawOverrides {
		definition, ok := runtimesettings.DefinitionByKey(key)
		if !ok {
			continue
		}

		plain := raw
		needsMigration := false
		if definition.Sensitive {
			if strings.HasPrefix(strings.TrimSpace(raw), "v2:") {
				plain, err = manager.DecryptString(raw, runtimeSettingSecretAAD(key))
				if err != nil {
					return nil, fmt.Errorf("decrypt runtime setting %s: %w", key, err)
				}
			} else {
				needsMigration = true
			}
		}

		normalized, normalizeErr := runtimesettings.NormalizeValue(definition, plain)
		if normalizeErr != nil {
			if definition.Sensitive {
				return nil, normalizeErr
			}
			// Retain the historical behavior for invalid non-secret rows: ignore
			// the bad override and resolve the environment/default value instead.
			continue
		}
		if needsMigration {
			// Compatibility migration for installs that persisted this value
			// before the setting was classified as sensitive.
			encrypted, encryptErr := manager.EncryptString(normalized, runtimeSettingSecretAAD(key))
			if encryptErr != nil {
				return nil, fmt.Errorf("migrate runtime setting %s: %w", key, encryptErr)
			}
			migrations[key] = encrypted
		}
		resolved[key] = normalized
	}
	if len(migrations) > 0 {
		if _, err := store.SaveRuntimeSettingOverrides(migrations); err != nil {
			return nil, fmt.Errorf("migrate sensitive runtime settings: %w", err)
		}
	}
	return resolved, nil
}

func runtimeSettingSecretAAD(key string) string {
	return runtimeSettingSecretAADPrefix + strings.TrimSpace(key)
}
