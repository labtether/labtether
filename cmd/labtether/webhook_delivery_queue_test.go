package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// A stuck webhook may hold only its own bounded share of memory, and must not
// stop another webhook from queueing events.
func TestWebhookQueueBoundsMemoryPerStuckWebhook(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	defer close(release)
	queues := newWebhookDeliveryQueues(ctx, func(context.Context, webhookDelivery) { <-release })

	large := json.RawMessage(`"` + strings.Repeat("x", maxWebhookEventDataBytes-2) + `"`)
	accepted := 0
	for i := 0; i < webhookQueueDepth; i++ {
		if queues.enqueue(webhookDelivery{hookID: "stuck", event: webhookDispatchEvent{Data: large}}) {
			accepted++
		}
	}
	// One event may already be with the blocked worker; the rest must fit the
	// per-webhook budget.
	if limit := webhookQueueMaxBytes/maxWebhookEventDataBytes + 1; accepted > limit {
		t.Fatalf("stuck webhook queued %d large events, want at most %d", accepted, limit)
	}
	if !queues.enqueue(webhookDelivery{hookID: "healthy", event: webhookDispatchEvent{Data: large}}) {
		t.Fatal("a stuck webhook blocked another webhook's queue")
	}
}
