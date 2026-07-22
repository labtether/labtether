package agentsettings

import "testing"

func TestEveryAgentSettingDefaultNormalizes(t *testing.T) {
	for _, definition := range AgentSettingDefinitions() {
		definition := definition
		t.Run(definition.Key, func(t *testing.T) {
			normalized, err := NormalizeAgentSettingValue(definition.Key, definition.DefaultValue)
			if err != nil {
				t.Fatalf("default %q is invalid: %v", definition.DefaultValue, err)
			}
			if normalized != definition.DefaultValue {
				t.Fatalf("default %q normalizes to %q; defaults must be canonical", definition.DefaultValue, normalized)
			}
		})
	}
}
