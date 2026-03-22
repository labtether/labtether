package runtimesettings

import "testing"

func TestServiceDiscoveryDefinitionsPresent(t *testing.T) {
	tests := []struct {
		key      string
		wantType ValueType
	}{
		{KeyServicesDiscoveryDefaultDockerEnabled, ValueTypeBool},
		{KeyServicesDiscoveryDefaultProxyEnabled, ValueTypeBool},
		{KeyServicesDiscoveryDefaultProxyTraefikEnabled, ValueTypeBool},
		{KeyServicesDiscoveryDefaultProxyCaddyEnabled, ValueTypeBool},
		{KeyServicesDiscoveryDefaultProxyNPMEnabled, ValueTypeBool},
		{KeyServicesDiscoveryDefaultPortScanEnabled, ValueTypeBool},
		{KeyServicesDiscoveryDefaultPortScanIncludeListening, ValueTypeBool},
		{KeyServicesDiscoveryDefaultPortScanPorts, ValueTypeString},
		{KeyServicesDiscoveryDefaultLANScanEnabled, ValueTypeBool},
		{KeyServicesDiscoveryDefaultLANScanCIDRs, ValueTypeString},
		{KeyServicesDiscoveryDefaultLANScanPorts, ValueTypeString},
		{KeyServicesDiscoveryDefaultLANScanMaxHosts, ValueTypeInt},
	}

	for _, tt := range tests {
		definition, ok := DefinitionByKey(tt.key)
		if !ok {
			t.Fatalf("expected runtime setting definition for %s", tt.key)
		}
		if definition.Type != tt.wantType {
			t.Fatalf("definition %s type = %s; want %s", tt.key, definition.Type, tt.wantType)
		}
	}
}

func TestSecurityOutboundAllowPrivateDefinitionPresent(t *testing.T) {
	definition, ok := DefinitionByKey(KeySecurityOutboundAllowPrivate)
	if !ok {
		t.Fatalf("expected runtime setting definition for %s", KeySecurityOutboundAllowPrivate)
	}
	if definition.Type != ValueTypeEnum {
		t.Fatalf("definition %s type = %s; want %s", definition.Key, definition.Type, ValueTypeEnum)
	}
	if definition.DefaultValue != "auto" {
		t.Fatalf("definition %s default = %q; want auto", definition.Key, definition.DefaultValue)
	}

	normalized, err := NormalizeValue(definition, "false")
	if err != nil {
		t.Fatalf("NormalizeValue returned error: %v", err)
	}
	if normalized != "false" {
		t.Fatalf("NormalizeValue returned %q; want false", normalized)
	}
}

func TestServiceDiscoveryStringRulesAllowEmpty(t *testing.T) {
	tests := []string{
		KeyServicesDiscoveryDefaultPortScanPorts,
		KeyServicesDiscoveryDefaultLANScanCIDRs,
		KeyServicesDiscoveryDefaultLANScanPorts,
	}

	for _, key := range tests {
		definition, ok := DefinitionByKey(key)
		if !ok {
			t.Fatalf("expected definition for %s", key)
		}
		normalized, err := NormalizeValue(definition, "")
		if err != nil {
			t.Fatalf("NormalizeValue(%s) returned error for empty value: %v", key, err)
		}
		if normalized != "" {
			t.Fatalf("NormalizeValue(%s) = %q; want empty string", key, normalized)
		}
	}
}

func TestServiceDiscoveryDefaultLANScanMaxHostsNormalization(t *testing.T) {
	definition, ok := DefinitionByKey(KeyServicesDiscoveryDefaultLANScanMaxHosts)
	if !ok {
		t.Fatalf("expected definition for %s", KeyServicesDiscoveryDefaultLANScanMaxHosts)
	}

	normalized, err := NormalizeValue(definition, "128")
	if err != nil {
		t.Fatalf("NormalizeValue returned error: %v", err)
	}
	if normalized != "128" {
		t.Fatalf("NormalizeValue returned %q; want 128", normalized)
	}

	if _, err := NormalizeValue(definition, "0"); err == nil {
		t.Fatalf("expected validation error for value below minimum")
	}
}
