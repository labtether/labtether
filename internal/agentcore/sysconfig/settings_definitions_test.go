package sysconfig

import "testing"

func TestServiceDiscoveryAgentSettingDefinitionsPresent(t *testing.T) {
	tests := []struct {
		key      string
		wantType AgentSettingType
	}{
		{SettingKeyServicesDiscoveryDockerEnabled, AgentSettingTypeBool},
		{SettingKeyServicesDiscoveryProxyEnabled, AgentSettingTypeBool},
		{SettingKeyServicesDiscoveryProxyTraefikEnabled, AgentSettingTypeBool},
		{SettingKeyServicesDiscoveryProxyCaddyEnabled, AgentSettingTypeBool},
		{SettingKeyServicesDiscoveryProxyNPMEnabled, AgentSettingTypeBool},
		{SettingKeyServicesDiscoveryPortScanEnabled, AgentSettingTypeBool},
		{SettingKeyServicesDiscoveryPortScanIncludeListening, AgentSettingTypeBool},
		{SettingKeyServicesDiscoveryPortScanPorts, AgentSettingTypeString},
		{SettingKeyServicesDiscoveryLANScanEnabled, AgentSettingTypeBool},
		{SettingKeyServicesDiscoveryLANScanCIDRs, AgentSettingTypeString},
		{SettingKeyServicesDiscoveryLANScanPorts, AgentSettingTypeString},
		{SettingKeyServicesDiscoveryLANScanMaxHosts, AgentSettingTypeInt},
	}

	for _, tt := range tests {
		definition, ok := AgentSettingDefinitionByKey(tt.key)
		if !ok {
			t.Fatalf("expected definition for %s", tt.key)
		}
		if definition.Type != tt.wantType {
			t.Fatalf("definition %s type = %s; want %s", tt.key, definition.Type, tt.wantType)
		}
	}
}

func TestNormalizeAgentSettingValueServiceDiscoveryPortList(t *testing.T) {
	normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryPortScanPorts, "8080, 443,8080")
	if err != nil {
		t.Fatalf("NormalizeAgentSettingValue returned error: %v", err)
	}
	if normalized != "443,8080" {
		t.Fatalf("NormalizeAgentSettingValue returned %q; want 443,8080", normalized)
	}

	if _, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryPortScanPorts, "443,nope"); err == nil {
		t.Fatalf("expected invalid port list to fail validation")
	}
}

func TestNormalizeAgentSettingValueServiceDiscoveryCIDRs(t *testing.T) {
	normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryLANScanCIDRs, "192.168.1.0/24,10.0.0.0/24")
	if err != nil {
		t.Fatalf("NormalizeAgentSettingValue returned error: %v", err)
	}
	if normalized != "10.0.0.0/24,192.168.1.0/24" {
		t.Fatalf("NormalizeAgentSettingValue returned %q; want sorted private CIDRs", normalized)
	}

	if _, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryLANScanCIDRs, "8.8.8.0/24"); err == nil {
		t.Fatalf("expected public CIDR to fail validation")
	}
}

func TestNormalizeAgentSettingValueDockerEndpointUnixSchemeCaseInsensitive(t *testing.T) {
	normalized, err := NormalizeAgentSettingValue(SettingKeyDockerEndpoint, "UNIX:///var/run/docker.sock")
	if err != nil {
		t.Fatalf("NormalizeAgentSettingValue returned error: %v", err)
	}
	if normalized != "unix:///var/run/docker.sock" {
		t.Fatalf("NormalizeAgentSettingValue returned %q; want unix:///var/run/docker.sock", normalized)
	}
}
