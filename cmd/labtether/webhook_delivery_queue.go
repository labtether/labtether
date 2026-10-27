package main

import (
	"context"
	"sync"
	"time"
)

const (
	// webhookQueueDepth bounds the events waiting for one webhook. A webhook
	// whose endpoint stops answering fills only its own queue; events for
	// other webhooks keep flowing.
	webhookQueueDepth = 64
	// webhookWorkerIdleTimeout stops a webhook's worker after it has had
	// nothing to send for a while, so deleted webhooks leave no goroutine.
	webhookWorkerIdleTimeout = time.Minute
)

type webhookDelivery struct {
	hookID string
	event  webhookDispatchEvent
}

// webhookDeliveryQueues gives each webhook its own bounded queue and worker,
// so deliveries to one webhook stay in order and a slow or unreachable
// endpoint cannot delay or drop events meant for the others.
type webhookDeliveryQueues struct {
	ctx     context.Context
	deliver func(context.Context, webhookDelivery)

	mu     sync.Mutex
	queues map[string]chan webhookDelivery
}

func newWebhookDeliveryQueues(ctx context.Context, deliver func(context.Context, webhookDelivery)) *webhookDeliveryQueues {
	return &webhookDeliveryQueues{ctx: ctx, deliver: deliver, queues: make(map[string]chan webhookDelivery)}
}

// enqueue never blocks. It reports false when the webhook's queue is full.
func (q *webhookDeliveryQueues) enqueue(delivery webhookDelivery) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	queue, ok := q.queues[delivery.hookID]
	if !ok {
		queue = make(chan webhookDelivery, webhookQueueDepth)
		q.queues[delivery.hookID] = queue
		go q.run(delivery.hookID, queue)
	}
	select {
	case queue <- delivery:
		return true
	default:
		return false
	}
}

func (q *webhookDeliveryQueues) run(hookID string, queue chan webhookDelivery) {
	idle := time.NewTimer(webhookWorkerIdleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-q.ctx.Done():
			return
		case delivery := <-queue:
			q.deliver(q.ctx, delivery)
			idle.Reset(webhookWorkerIdleTimeout)
		case <-idle.C:
			// Senders hold the lock while they queue, so an empty queue seen
			// under the lock stays empty until this worker is removed.
			q.mu.Lock()
			if len(queue) == 0 {
				delete(q.queues, hookID)
				q.mu.Unlock()
				return
			}
			q.mu.Unlock()
			idle.Reset(webhookWorkerIdleTimeout)
		}
	}
}
