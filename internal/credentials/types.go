package credentials

import "time"

const (
	KindSSHPassword        = "ssh_password"
	KindSSHPrivateKey      = "ssh_private_key"
	KindHubSSHIdentity     = "hub_ssh_identity"
	KindVNCPassword        = "vnc_password"
	KindProxmoxAPIToken    = "proxmox_api_token" // #nosec G101 -- credential kind identifier, not secret material
	KindProxmoxPassword    = "proxmox_password"  // #nosec G101 -- credential kind identifier, not secret material
	KindPBSAPIToken        = "pbs_api_token"     // #nosec G101 -- credential kind identifier, not secret material
	KindPortainerAPIKey    = "portainer_api_key" // #nosec G101 -- credential kind identifier, not secret material
	KindTrueNASAPIKey      = "truenas_api_key"   // #nosec G101 -- credential kind identifier, not secret material
	KindHomeAssistantToken = "homeassistant_token"
	KindTelnetPassword     = "telnet_password"
	KindRDPPassword        = "rdp_password"
	KindFTPPassword        = "ftp_password"
	KindSMBCredentials     = "smb_credentials"    // #nosec G101 -- credential kind identifier, not secret material
	KindWebDAVCredentials  = "webdav_credentials" // #nosec G101 -- credential kind identifier, not secret material
)

// Profile stores credential metadata and encrypted payloads used by terminal execution.
// SecretCiphertext/PassphraseCiphertext are storage-only fields and must never be exposed to clients.
type Profile struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Kind                 string            `json:"kind"`
	Username             string            `json:"username,omitempty"`
	Description          string            `json:"description,omitempty"`
	Status               string            `json:"status,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	RotatedAt            *time.Time        `json:"rotated_at,omitempty"`
	LastUsedAt           *time.Time        `json:"last_used_at,omitempty"`
	ExpiresAt            *time.Time        `json:"expires_at,omitempty"`
	SecretCiphertext     string            `json:"-"`
	PassphraseCiphertext string            `json:"-"`
}

type CreateProfileRequest struct {
	Name        string            `json:"name"`
	Kind        string            `json:"kind"`
	Username    string            `json:"username,omitempty"`
	Description string            `json:"description,omitempty"`
	Secret      string            `json:"secret"` // #nosec G117 -- API DTO field carries runtime secret input, not a hardcoded credential.
	Passphrase  string            `json:"passphrase,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

type RotateProfileRequest struct {
	Secret      string     `json:"secret"` // #nosec G117 -- API DTO field carries runtime secret input, not a hardcoded credential.
	Passphrase  string     `json:"passphrase,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	KeepEnabled bool       `json:"keep_enabled,omitempty"`
}

// AssetTerminalConfig binds a managed asset to an SSH endpoint and credential profile.
type AssetTerminalConfig struct {
	AssetID             string    `json:"asset_id"`
	Host                string    `json:"host"`
	Port                int       `json:"port"`
	Username            string    `json:"username,omitempty"`
	StrictHostKey       bool      `json:"strict_host_key"`
	HostKey             string    `json:"host_key,omitempty"`
	CredentialProfileID string    `json:"credential_profile_id,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type UpdateAssetTerminalConfigRequest struct {
	Host                string `json:"host"`
	Port                int    `json:"port,omitempty"`
	Username            string `json:"username,omitempty"`
	StrictHostKey       *bool  `json:"strict_host_key,omitempty"`
	HostKey             string `json:"host_key,omitempty"`
	CredentialProfileID string `json:"credential_profile_id,omitempty"`
}

// AssetDesktopConfig binds a managed asset to VNC credentials for auto-fill.
type AssetDesktopConfig struct {
	AssetID             string    `json:"asset_id"`
	VNCPort             int       `json:"vnc_port"`
	CredentialProfileID string    `json:"credential_profile_id,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
}
