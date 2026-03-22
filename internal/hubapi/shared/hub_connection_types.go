package shared

// HubConnectionCandidate represents one possible way a remote agent can
// connect back to the hub (LAN IP, Tailscale IP, external URL, etc.).
type HubConnectionCandidate struct {
	Kind              string `json:"kind"`
	Label             string `json:"label"`
	Host              string `json:"host"`
	HubURL            string `json:"hub_url"`
	WSURL             string `json:"ws_url"`
	PreferredReason   string `json:"preferred_reason,omitempty"`
	TrustMode         string `json:"trust_mode,omitempty"`
	BootstrapURL      string `json:"bootstrap_url,omitempty"`
	BootstrapStrategy string `json:"bootstrap_strategy,omitempty"`
}

// HubConnectionSelection holds the resolved hub connectivity for an
// agent enrollment or discovery response.
type HubConnectionSelection struct {
	HubURL     string
	WSURL      string
	Candidates []HubConnectionCandidate
}

// HubSSHIdentity holds the hub's SSH keypair info for auto-provisioning
// SSH keys to agents.
type HubSSHIdentity struct {
	ProfileID string
	PublicKey string // OpenSSH format: "ssh-ed25519 AAAA... labtether-hub"
	KeyType   string // "ed25519" or "rsa"
}
