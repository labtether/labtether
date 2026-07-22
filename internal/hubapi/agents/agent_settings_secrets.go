package agents

import (
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/agentsettings"
)

const encryptedAgentSettingPrefix = "secret:v1:"

func agentSettingSecretAAD(storeKey string) string {
	return "agent-setting:" + storeKey
}

func (d *Deps) encodeAgentSettingForStore(settingKey, storeKey, value string) (string, error) {
	definition, ok := agentsettings.AgentSettingDefinitionByKey(settingKey)
	if !ok || !definition.Sensitive || value == "" {
		return value, nil
	}
	if d.SecretsManager == nil {
		return "", fmt.Errorf("secret storage is unavailable for %s", settingKey)
	}
	ciphertext, err := d.SecretsManager.EncryptString(value, agentSettingSecretAAD(storeKey))
	if err != nil {
		return "", fmt.Errorf("encrypt %s: %w", settingKey, err)
	}
	return encryptedAgentSettingPrefix + ciphertext, nil
}

func (d *Deps) decodeAgentSettingFromStore(settingKey, storeKey, stored string) (string, error) {
	definition, ok := agentsettings.AgentSettingDefinitionByKey(settingKey)
	if !ok || !definition.Sensitive || stored == "" {
		return stored, nil
	}
	if !strings.HasPrefix(stored, encryptedAgentSettingPrefix) {
		return stored, nil // legacy plaintext; migration is handled separately.
	}
	if d.SecretsManager == nil {
		return "", fmt.Errorf("secret storage is unavailable for %s", settingKey)
	}
	plain, err := d.SecretsManager.DecryptString(strings.TrimPrefix(stored, encryptedAgentSettingPrefix), agentSettingSecretAAD(storeKey))
	if err != nil {
		return "", fmt.Errorf("decrypt %s: %w", settingKey, err)
	}
	return plain, nil
}

func sensitiveAgentSettingKeyForStoreKey(storeKey string) (string, bool) {
	if !strings.HasPrefix(storeKey, AgentSettingsStorePrefix) {
		return "", false
	}
	for _, definition := range agentsettings.AgentSettingDefinitions() {
		if definition.Sensitive && strings.HasSuffix(storeKey, "."+definition.Key) {
			return definition.Key, true
		}
	}
	return "", false
}

// MigrateSensitiveAgentSettingOverrides encrypts legacy plaintext values in a
// single store update. It is safe to call repeatedly.
func (d *Deps) MigrateSensitiveAgentSettingOverrides(overrides map[string]string) error {
	updates := map[string]string{}
	for storeKey, stored := range overrides {
		settingKey, sensitive := sensitiveAgentSettingKeyForStoreKey(storeKey)
		if !sensitive || stored == "" || strings.HasPrefix(stored, encryptedAgentSettingPrefix) {
			continue
		}
		encoded, err := d.encodeAgentSettingForStore(settingKey, storeKey, stored)
		if err != nil {
			return err
		}
		updates[storeKey] = encoded
	}
	if len(updates) == 0 {
		return nil
	}
	if d.RuntimeStore == nil {
		return fmt.Errorf("runtime settings store is unavailable")
	}
	if _, err := d.RuntimeStore.SaveRuntimeSettingOverrides(updates); err != nil {
		return err
	}
	for key, value := range updates {
		overrides[key] = value
	}
	return nil
}

func redactSensitiveAgentSettingValues(values map[string]string) map[string]string {
	out := CloneAgentSettingValues(values)
	for key := range out {
		if definition, ok := agentsettings.AgentSettingDefinitionByKey(key); ok && definition.Sensitive {
			delete(out, key)
		}
	}
	return out
}
