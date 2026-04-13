package webhooks

import "time"

// Webhook represents a registered webhook subscription.
type Webhook struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	URL              string     `json:"url"`
	Secret           string     `json:"-"`
	SecretCiphertext string     `json:"-"`
	Events           []string   `json:"events"`
	Enabled          bool       `json:"enabled"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	LastTriggeredAt  *time.Time `json:"last_triggered_at,omitempty"`
}

// CreateRequest holds the fields required to register a new webhook.
type CreateRequest struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"` // #nosec G117 -- Webhook secret is provided at runtime, not hardcoded in source.
	Events []string `json:"events"`
}

// UpdateRequest holds optional fields for partially updating a webhook.
type UpdateRequest struct {
	Name    *string   `json:"name,omitempty"`
	URL     *string   `json:"url,omitempty"`
	Secret  *string   `json:"secret,omitempty"` // #nosec G117 -- Webhook secret is provided at runtime, not hardcoded in source.
	Events  *[]string `json:"events,omitempty"`
	Enabled *bool     `json:"enabled,omitempty"`
}
