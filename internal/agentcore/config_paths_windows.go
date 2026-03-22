//go:build windows

package agentcore

const (
	defaultConfigDir             = `C:\ProgramData\LabTether`
	defaultTokenFile             = `C:\ProgramData\LabTether\agent-token`
	defaultSettingsFile          = `C:\ProgramData\LabTether\agent-config.json`
	defaultDeviceKeyFile         = `C:\ProgramData\LabTether\device-key`
	defaultDevicePublicKeyFile   = `C:\ProgramData\LabTether\device-key.pub`
	defaultDeviceFingerprintFile = `C:\ProgramData\LabTether\device-fingerprint`
)
