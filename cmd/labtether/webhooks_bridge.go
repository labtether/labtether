package main

import webhookspkg "github.com/labtether/labtether/internal/hubapi/webhookspkg"

// buildWebhooksDeps constructs the webhookspkg.Deps from the apiServer's fields.
func (s *apiServer) buildWebhooksDeps() *webhookspkg.Deps {
	return &webhookspkg.Deps{
		WebhookStore: s.webhookStore,
		AuditStore:   s.auditStore,
	}
}

// ensureWebhooksDeps returns the webhooks deps, creating and caching on first call.
func (s *apiServer) ensureWebhooksDeps() *webhookspkg.Deps {
	if s.webhooksDeps != nil {
		return s.webhooksDeps
	}
	d := s.buildWebhooksDeps()
	s.webhooksDeps = d
	return d
}
