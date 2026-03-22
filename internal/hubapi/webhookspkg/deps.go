package webhookspkg

import "github.com/labtether/labtether/internal/persistence"

// Deps holds the dependencies for webhook handlers.
type Deps struct {
	WebhookStore persistence.WebhookStore
	AuditStore   persistence.AuditStore
}
