package protocols

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// Protocol kind constants for manual device connections.
const (
	ProtocolSSH    = "ssh"
	ProtocolTelnet = "telnet"
	ProtocolVNC    = "vnc"
	ProtocolRDP    = "rdp"
	ProtocolARD    = "ard"
)

// ValidProtocols is the set of supported protocol identifiers.
var ValidProtocols = map[string]struct{}{
	ProtocolSSH:    {},
	ProtocolTelnet: {},
	ProtocolVNC:    {},
	ProtocolRDP:    {},
	ProtocolARD:    {},
}

// DefaultPort returns the conventional default port for a protocol.
// Returns 0 for unknown protocols.
func DefaultPort(protocol string) int {
	switch protocol {
	case ProtocolSSH:
		return 22
	case ProtocolTelnet:
		return 23
	case ProtocolVNC, ProtocolARD:
		return 5900
	case ProtocolRDP:
		return 3389
	default:
		return 0
	}
}

// ProtocolConfig stores the configuration for a single manual device protocol endpoint.
type ProtocolConfig struct {
	ID                  string          `json:"id"`
	AssetID             string          `json:"asset_id"`
	Protocol            string          `json:"protocol"`
	Host                string          `json:"host"`
	Port                int             `json:"port"`
	Username            string          `json:"username,omitempty"`
	CredentialProfileID string          `json:"credential_profile_id,omitempty"`
	Enabled             bool            `json:"enabled"`
	LastTestedAt        *time.Time      `json:"last_tested_at,omitempty"`
	TestStatus          string          `json:"test_status,omitempty"`
	TestError           string          `json:"test_error,omitempty"`
	Config              json.RawMessage `json:"config,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// SSHConfig holds SSH-specific connection options.
type SSHConfig struct {
	StrictHostKey   bool   `json:"strict_host_key,omitempty"`
	HostKey         string `json:"host_key,omitempty"`
	HubKeyInstalled bool   `json:"hub_key_installed,omitempty"`
}

// VNCConfig holds VNC-specific connection options.
type VNCConfig struct {
	DisplayNumber int `json:"display_number,omitempty"`
}

// RDPConfig holds RDP-specific connection options.
type RDPConfig struct {
	Domain     string `json:"domain,omitempty"`
	NLAEnabled bool   `json:"nla_enabled,omitempty"`
}

// ARDConfig holds Apple Remote Desktop-specific connection options.
type ARDConfig struct {
	AppleDH bool `json:"apple_dh,omitempty"`
}

// TestResult captures the outcome of a protocol connectivity test.
type TestResult struct {
	Success   bool   `json:"success"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
	Message   string `json:"message,omitempty"`
}

// ValidateProtocol returns an error if protocol is not a known supported value.
func ValidateProtocol(protocol string) error {
	if _, ok := ValidProtocols[protocol]; !ok {
		return fmt.Errorf("unsupported protocol %q: must be one of ssh, telnet, vnc, rdp, ard", protocol)
	}
	return nil
}

// ValidatePort returns an error if port is outside the valid TCP/UDP range.
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d is out of range: must be between 1 and 65535", port)
	}
	return nil
}

// ValidateProtocolConfig decodes and validates the raw JSON config for a given protocol.
// Unknown fields are rejected. Telnet requires no config fields.
func ValidateProtocolConfig(protocol string, raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	switch protocol {
	case ProtocolSSH:
		var cfg SSHConfig
		return strictUnmarshal(raw, &cfg)
	case ProtocolVNC:
		var cfg VNCConfig
		return strictUnmarshal(raw, &cfg)
	case ProtocolRDP:
		var cfg RDPConfig
		return strictUnmarshal(raw, &cfg)
	case ProtocolARD:
		var cfg ARDConfig
		return strictUnmarshal(raw, &cfg)
	case ProtocolTelnet:
		// Telnet has no configurable fields; any supplied config is rejected.
		var discard map[string]json.RawMessage
		if err := strictUnmarshal(raw, &discard); err != nil {
			return err
		}
		if len(discard) > 0 {
			return fmt.Errorf("telnet protocol does not accept config fields")
		}
		return nil
	default:
		return fmt.Errorf("unsupported protocol %q", protocol)
	}
}

// strictUnmarshal decodes raw JSON into target while rejecting unknown fields.
func strictUnmarshal(raw json.RawMessage, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}
