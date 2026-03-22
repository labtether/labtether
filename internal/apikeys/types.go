package apikeys

import "time"

// GeneratedKey is returned from GenerateKey. Raw is shown once to the user;
// Prefix and Hash are stored.
type GeneratedKey struct {
	Raw    string // full key: lt_<prefix>_<secret>
	Prefix string // 4-char identifier
	Hash   string // SHA-256 hex digest of Raw
}

// APIKey is the stored representation of a key (never contains the raw secret).
type APIKey struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Prefix        string     `json:"prefix"`
	SecretHash    string     `json:"-"`
	Role          string     `json:"role"`
	Scopes        []string   `json:"scopes"`
	AllowedAssets []string   `json:"allowed_assets,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
}

// CreateKeyRequest is the input for creating a new API key.
type CreateKeyRequest struct {
	Name          string     `json:"name"`
	Role          string     `json:"role"`
	Scopes        []string   `json:"scopes"`
	AllowedAssets []string   `json:"allowed_assets,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

// KeyInfo is the public view of a key (for listing).
type KeyInfo struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Prefix        string     `json:"prefix"`
	Role          string     `json:"role"`
	Scopes        []string   `json:"scopes"`
	AllowedAssets []string   `json:"allowed_assets,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
}
