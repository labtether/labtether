package main

import "net/http"

func (s *apiServer) handleV2Webhooks(w http.ResponseWriter, r *http.Request) {
	s.ensureWebhooksDeps().HandleV2Webhooks(w, r)
}

func (s *apiServer) handleV2WebhookActions(w http.ResponseWriter, r *http.Request) {
	s.ensureWebhooksDeps().HandleV2WebhookActions(w, r)
}
