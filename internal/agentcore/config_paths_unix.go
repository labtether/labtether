//go:build !windows

package agentcore

const (
	defaultConfigDir             = "/etc/labtether"
	defaultTokenFile             = "/etc/labtether/agent-token" // #nosec G101 -- Filesystem path constant, not a credential.
	defaultSettingsFile          = "/etc/labtether/agent-config.json"
	defaultDeviceKeyFile         = "/etc/labtether/device-key"
	defaultDevicePublicKeyFile   = "/etc/labtether/device-key.pub"
	defaultDeviceFingerprintFile = "/etc/labtether/device-fingerprint"
)
