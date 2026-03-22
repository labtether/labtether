package persistence

import (
	"context"
	"fmt"
	"sync"

	"github.com/labtether/labtether/internal/webhooks"
)

type memoryWebhookStore struct {
	mu   sync.RWMutex
	data map[string]webhooks.Webhook
}

// NewMemoryWebhookStore returns an in-memory implementation of WebhookStore.
func NewMemoryWebhookStore() WebhookStore {
	return &memoryWebhookStore{data: make(map[string]webhooks.Webhook)}
}

func (m *memoryWebhookStore) CreateWebhook(_ context.Context, wh webhooks.Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.data[wh.ID]; exists {
		return fmt.Errorf("webhook already exists: %s", wh.ID)
	}
	m.data[wh.ID] = wh
	return nil
}

func (m *memoryWebhookStore) GetWebhook(_ context.Context, id string) (webhooks.Webhook, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	wh, ok := m.data[id]
	return wh, ok, nil
}

func (m *memoryWebhookStore) ListWebhooks(_ context.Context) ([]webhooks.Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]webhooks.Webhook, 0, len(m.data))
	for _, wh := range m.data {
		result = append(result, wh)
	}
	return result, nil
}

func (m *memoryWebhookStore) UpdateWebhook(_ context.Context, id string, name *string, url *string, events *[]string, enabled *bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	wh, ok := m.data[id]
	if !ok {
		return fmt.Errorf("webhook not found: %s", id)
	}
	if name != nil {
		wh.Name = *name
	}
	if url != nil {
		wh.URL = *url
	}
	if events != nil {
		wh.Events = *events
	}
	if enabled != nil {
		wh.Enabled = *enabled
	}
	m.data[id] = wh
	return nil
}

func (m *memoryWebhookStore) DeleteWebhook(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, id)
	return nil
}
