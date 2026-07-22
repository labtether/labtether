package groupprofiles

import (
	"strings"
	"testing"
)

func TestNormalizeConfigRejectsSecretAndUnboundedFields(t *testing.T) {
	for _, config := range []map[string]any{
		{"webrtc_turn_pass": "secret"},
		{"smtp_pass": "secret"},
		{"expected_asset_count": MaxExpectedAssetCount + 1},
		{"required_platforms": []string{strings.Repeat("x", MaxPlatformNameLength+1)}},
		{"min_online_percent": 101},
	} {
		if _, err := NormalizeConfig(config); err == nil {
			t.Fatalf("expected config to be rejected: %#v", config)
		}
	}
}

func TestNormalizeConfigCanonicalizesSupportedSchema(t *testing.T) {
	config, err := NormalizeConfig(map[string]any{
		"expected_asset_count": float64(2),
		"required_platforms":   []any{" Linux ", "linux", "Windows"},
		"min_online_percent":   float64(90),
	})
	if err != nil {
		t.Fatal(err)
	}
	platforms, ok := config["required_platforms"].([]string)
	if !ok || len(platforms) != 2 || platforms[0] != "linux" || platforms[1] != "windows" {
		t.Fatalf("unexpected platforms: %#v", config["required_platforms"])
	}
}

func TestSanitizeLegacyConfigDropsUnknownSecretFields(t *testing.T) {
	config := SanitizeLegacyConfig(map[string]any{
		"expected_asset_count": 2,
		"webrtc_turn_pass":     "must-not-return",
	})
	if _, exposed := config["webrtc_turn_pass"]; exposed {
		t.Fatal("legacy secret-bearing field was not removed")
	}
	if config["expected_asset_count"] != 2 {
		t.Fatalf("supported field was not preserved: %#v", config)
	}
}
