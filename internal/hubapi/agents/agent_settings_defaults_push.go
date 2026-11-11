package agents

import (
	"github.com/labtether/labtether/internal/agentsettings"
)

// PushAgentDefaultsToConnected sends every connected agent the values it
// should now run with after Hub-wide agent defaults changed, so a change in
// Settings reaches agents without waiting for them to reconnect. changedKeys
// and resetKeys are runtime setting keys; nothing is sent unless one of them
// is a Hub-wide agent default.
//
// Agents keep a pushed value over their own install-time value, and the
// connect-time push sends only values the Hub has set. So after a reset the
// agent is sent the setting's default, unless the device has its own value;
// otherwise it would keep the old Hub-wide value for good.
func (d *Deps) PushAgentDefaultsToConnected(changedKeys, resetKeys []string) {
	if d == nil || d.AgentMgr == nil {
		return
	}
	touched := map[string]bool{}
	for _, key := range changedKeys {
		touched[key] = true
	}
	reset := map[string]bool{}
	for _, key := range resetKeys {
		touched[key] = true
		reset[key] = true
	}
	resetDefaults := map[string]string{}
	relevant := false
	for _, definition := range agentsettings.AgentSettingDefinitions() {
		globalKey, ok := AgentSettingGlobalDefaultKey(definition.Key)
		if !ok || !touched[globalKey] || definition.LocalOnly {
			continue
		}
		relevant = true
		if reset[globalKey] {
			resetDefaults[definition.Key] = definition.DefaultValue
		}
	}
	if !relevant {
		return
	}
	for _, assetID := range d.AgentMgr.ConnectedAssets() {
		values, err := d.CollectConfiguredAgentSettingValues(assetID)
		if err != nil {
			continue
		}
		for key := range values {
			if definition, ok := agentsettings.AgentSettingDefinitionByKey(key); ok && definition.LocalOnly {
				delete(values, key)
			}
		}
		for key, value := range resetDefaults {
			if _, own := values[key]; !own {
				values[key] = value
			}
		}
		if len(values) > 0 {
			d.PushAgentSettingsApply(assetID, values)
		}
	}
}
