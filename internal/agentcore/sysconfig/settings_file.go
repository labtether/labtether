package sysconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const agentSettingsFileVersion = 1

type agentSettingsFile struct {
	Version   int               `json:"version"`
	Values    map[string]string `json:"values"`
	UpdatedAt string            `json:"updated_at"`
}

func LoadAgentSettingsFile(path string) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- Settings path is a controlled runtime file chosen by the sysconfig package.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	var payload agentSettingsFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	values := make(map[string]string)
	for key, raw := range payload.Values {
		definition, ok := AgentSettingDefinitionByKey(key)
		if !ok {
			continue
		}
		normalized, err := NormalizeAgentSettingValue(definition.Key, raw)
		if err != nil {
			continue
		}
		values[definition.Key] = normalized
	}
	return values, nil
}

func SaveAgentSettingsFile(path string, values map[string]string) error {
	if path == "" {
		return errors.New("settings file path is required")
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		definition, ok := AgentSettingDefinitionByKey(key)
		if !ok {
			return errors.New("unknown agent setting key: " + key)
		}
		nv, err := NormalizeAgentSettingValue(definition.Key, value)
		if err != nil {
			return err
		}
		normalized[definition.Key] = nv
	}

	payload := agentSettingsFile{
		Version:   agentSettingsFileVersion,
		Values:    normalized,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	// Atomic write: temp file + rename to prevent torn writes from
	// concurrent settings updates.
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
