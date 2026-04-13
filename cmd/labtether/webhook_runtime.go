package main

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/webhooks"
)

type webhookDispatchEvent struct {
	EventType string
	Data      any
	Timestamp time.Time
}

func (s *apiServer) enqueueWebhookEvent(eventType string, data any, at time.Time) {
	if s == nil || s.webhookEventCh == nil {
		return
	}
	event := webhookDispatchEvent{
		EventType: strings.TrimSpace(eventType),
		Data:      data,
		Timestamp: at.UTC(),
	}
	if event.EventType == "" {
		return
	}
	select {
	case s.webhookEventCh <- event:
	default:
		log.Printf("webhooks: dropping event %q because relay queue is full", event.EventType)
	}
}

func (s *apiServer) invalidateWebhookCache() {
	if s == nil {
		return
	}
	s.webhookCacheMu.Lock()
	s.webhookCacheDirty = true
	s.webhookCacheMu.Unlock()
}

func (s *apiServer) runWebhookRelay(ctx context.Context) {
	if s == nil || s.webhookStore == nil || s.webhookEventCh == nil {
		return
	}
	adapter, ok := s.notificationDispatcher.Adapters[notifications.ChannelTypeWebhook].(*notifications.WebhookAdapter)
	if !ok || adapter == nil {
		log.Printf("webhooks: webhook adapter not configured; relay disabled")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-s.webhookEventCh:
			s.dispatchWebhookEvent(ctx, adapter, event)
		}
	}
}

func (s *apiServer) dispatchWebhookEvent(ctx context.Context, adapter *notifications.WebhookAdapter, event webhookDispatchEvent) {
	hooks, err := s.listDispatchWebhooks(ctx)
	if err != nil {
		log.Printf("webhooks: load failed for event %q: %v", event.EventType, err)
		return
	}
	if len(hooks) == 0 {
		return
	}

	payload := map[string]any{
		"type": event.EventType,
		"data": event.Data,
		"ts":   event.Timestamp.UTC().Format(time.RFC3339),
	}
	for _, wh := range hooks {
		if !wh.Enabled || !webhookMatchesEvent(wh, event.EventType) {
			continue
		}

		config := map[string]any{
			"url":        wh.URL,
			"event_type": event.EventType,
			"timestamp":  payload["ts"],
		}
		secret, err := s.runtimeWebhookSecret(ctx, wh)
		if err != nil {
			log.Printf("webhooks: %s secret unavailable: %v", wh.ID, err)
			continue
		}
		if secret != "" {
			config["secret"] = secret
		}
		if err := adapter.Send(ctx, config, payload); err != nil {
			log.Printf("webhooks: %s delivery failed for %q: %v", wh.ID, event.EventType, err)
			continue
		}
		if err := s.webhookStore.MarkWebhookTriggered(ctx, wh.ID, event.Timestamp); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			log.Printf("webhooks: %s trigger timestamp update failed: %v", wh.ID, err)
		}
		s.setWebhookCacheLastTriggered(wh.ID, event.Timestamp)
	}
}

func (s *apiServer) listDispatchWebhooks(ctx context.Context) ([]webhooks.Webhook, error) {
	s.webhookCacheMu.RLock()
	if s.webhookCacheLoaded && !s.webhookCacheDirty {
		cached := append([]webhooks.Webhook(nil), s.webhookCache...)
		s.webhookCacheMu.RUnlock()
		return cached, nil
	}
	s.webhookCacheMu.RUnlock()

	hooks, err := s.webhookStore.ListWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if hooks == nil {
		hooks = []webhooks.Webhook{}
	}

	s.webhookCacheMu.Lock()
	s.webhookCache = append([]webhooks.Webhook(nil), hooks...)
	s.webhookCacheLoaded = true
	s.webhookCacheDirty = false
	s.webhookCacheMu.Unlock()
	return hooks, nil
}

func (s *apiServer) setWebhookCacheLastTriggered(id string, at time.Time) {
	s.webhookCacheMu.Lock()
	defer s.webhookCacheMu.Unlock()
	for i := range s.webhookCache {
		if s.webhookCache[i].ID != id {
			continue
		}
		triggeredAt := at.UTC()
		s.webhookCache[i].LastTriggeredAt = &triggeredAt
		return
	}
}

func (s *apiServer) runtimeWebhookSecret(ctx context.Context, wh webhooks.Webhook) (string, error) {
	if strings.TrimSpace(wh.SecretCiphertext) != "" {
		if s.secretsManager == nil {
			return "", errors.New("secrets manager is not configured")
		}
		return s.secretsManager.DecryptString(wh.SecretCiphertext, wh.ID)
	}
	return strings.TrimSpace(wh.Secret), nil
}

func webhookMatchesEvent(wh webhooks.Webhook, eventType string) bool {
	if len(wh.Events) == 0 {
		return true
	}
	for _, event := range wh.Events {
		if event == eventType {
			return true
		}
	}
	return false
}
