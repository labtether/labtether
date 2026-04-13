package webhookspkg

import "github.com/labtether/labtether/internal/persistence"

// SecretsManagerInterface is the subset of secrets.Manager used by webhook handlers.
type SecretsManagerInterface interface {
	EncryptString(plain string, aad string) (string, error)
}

// Deps holds the dependencies for webhook handlers.
type Deps struct {
	WebhookStore           persistence.WebhookStore
	AuditStore             persistence.AuditStore
	SecretsManager         SecretsManagerInterface
	InvalidateWebhookCache func()
}
