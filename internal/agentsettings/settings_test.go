package agentsettings

import (
	"strings"
	"testing"
)

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

func TestNormalizeDockerEndpointAcceptsCanonicalNpipe(t *testing.T) {
	const endpoint = "npipe:////./pipe/docker_engine"
	normalized, err := NormalizeAgentSettingValue(SettingKeyDockerEndpoint, endpoint)
	if err != nil {
		t.Fatalf("canonical npipe endpoint rejected: %v", err)
	}
	if normalized != endpoint {
		t.Fatalf("normalized endpoint = %q, want %q", normalized, endpoint)
	}
}

func TestNormalizeDockerEndpointRejectsNonCanonicalNpipeForms(t *testing.T) {
	tests := map[string]string{
		"empty pipe":          "npipe:////./pipe/",
		"relative form":       "npipe://./pipe/docker_engine",
		"remote host":         "npipe:////server/pipe/docker_engine",
		"UNC backslashes":     `npipe:\\\\server\\pipe\\docker_engine`,
		"uppercase scheme":    "NPIPE:////./pipe/docker_engine",
		"traversal":           "npipe:////./pipe/docker..engine",
		"dot only":            "npipe:////./pipe/.",
		"leading punctuation": "npipe:////./pipe/_docker_engine",
		"trailing dot":        "npipe:////./pipe/docker_engine.",
		"slash":               "npipe:////./pipe/docker/engine",
		"backslash":           `npipe:////./pipe/docker\\engine`,
		"percent confusable":  "npipe:////./pipe/%64ocker_engine",
		"unicode confusable":  "npipe:////./pipe/dockеr_engine",
		"control":             "npipe:////./pipe/docker\nengine",
		"query":               "npipe:////./pipe/docker_engine?x=1",
		"fragment":            "npipe:////./pipe/docker_engine#x",
		"credential":          "npipe:////./pipe/user@docker_engine",
		"oversize":            "npipe:////./pipe/" + strings.Repeat("a", dockerNpipeNameMaxBytes+1),
		"unsupported scheme":  "tcp://127.0.0.1:2375",
	}
	for name, endpoint := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeAgentSettingValue(SettingKeyDockerEndpoint, endpoint); err == nil {
				t.Fatalf("unsafe endpoint %q was accepted", endpoint)
			}
		})
	}
}
