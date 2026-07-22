package enrollment

import "time"

const (
	DeviceProofVersionV1 = "v1"
	DeviceProofVersionV2 = "v2"
)

// EnrollmentToken represents a short-lived token that agents use to self-register.
type EnrollmentToken struct {
	ID        string     `json:"id"`
	Label     string     `json:"label"`
	ExpiresAt time.Time  `json:"expires_at"`
	MaxUses   int        `json:"max_uses"`
	UseCount  int        `json:"use_count"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// AgentToken represents a per-agent API token issued during enrollment.
type AgentToken struct {
	ID          string     `json:"id"`
	AssetID     string     `json:"asset_id"`
	Status      string     `json:"status"` // "pending", "active", or "revoked"
	EnrolledVia string     `json:"enrolled_via,omitempty"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// EnrollRequest is the payload an agent sends to POST /api/v1/enroll.
type EnrollRequest struct {
	EnrollmentToken    string `json:"enrollment_token"`
	Hostname           string `json:"hostname"`
	Platform           string `json:"platform"`
	GroupID            string `json:"group_id,omitempty"`
	DeviceKeyAlg       string `json:"device_key_algorithm,omitempty"`
	DevicePublicKey    string `json:"device_public_key,omitempty"`
	DeviceFingerprint  string `json:"device_fingerprint,omitempty"`
	DeviceSignature    string `json:"device_signature,omitempty"`
	DeviceProofVersion string `json:"device_proof_version,omitempty"`
}

// EnrollResponse is returned to the agent after successful enrollment.
type EnrollResponse struct {
	AgentToken string `json:"agent_token"`
	AssetID    string `json:"asset_id"`
	// GroupID is always emitted so an agent can distinguish a canonical
	// unplaced asset from an older Hub that did not return group placement.
	GroupID   string `json:"group_id"`
	HubWSURL  string `json:"hub_ws_url"`
	HubAPIURL string `json:"hub_api_url"`
	CACertPEM string `json:"ca_cert_pem,omitempty"`
}

// CreateTokenRequest is the admin request to generate a new enrollment token.
type CreateTokenRequest struct {
	Label    string `json:"label"`
	TTLHours int    `json:"ttl_hours"`
	MaxUses  int    `json:"max_uses"`
}
