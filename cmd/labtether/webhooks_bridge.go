package main

import webhookspkg "github.com/labtether/labtether/internal/hubapi/webhookspkg"

// buildWebhooksDeps constructs the webhookspkg.Deps from the apiServer's fields.
func (s *apiServer) buildWebhooksDeps() *webhookspkg.Deps {
	return &webhookspkg.Deps{
		WebhookStore:           s.webhookStore,
		AuditStore:             s.auditStore,
		SecretsManager:         s.secretsManager,
		InvalidateWebhookCache: s.invalidateWebhookCache,
	}
}

// ensureWebhooksDeps returns the webhooks deps, creating and caching on first call.
func (s *apiServer) ensureWebhooksDeps() *webhookspkg.Deps {
	s.webhooksDepsOnce.Do(func() {
		s.webhooksDeps = s.buildWebhooksDeps()
	})
	return s.webhooksDeps
}
