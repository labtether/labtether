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
	// webhookQueueMaxBytes bounds the event data waiting for one webhook, and
	// webhookQueuesMaxBytes the data waiting across all of them. The total
	// matches what the single relay channel could hold before (256 events of
	// up to 256 KiB), so stuck endpoints cannot grow the Hub's memory use.
	webhookQueueMaxBytes  = 4 << 20
	webhookQueuesMaxBytes = 64 << 20
	// webhookWorkerIdleTimeout stops a webhook's worker after it has had
	// nothing to send for a while, so deleted webhooks leave no goroutine.
	webhookWorkerIdleTimeout = time.Minute
)

type webhookDelivery struct {
	hookID string
	event  webhookDispatchEvent
}

func (d webhookDelivery) size() int64 { return int64(len(d.event.Data)) }

type webhookQueue struct {
	deliveries chan webhookDelivery
	bytes      int64
}

// webhookDeliveryQueues gives each webhook its own bounded queue and worker,
// so deliveries to one webhook stay in order and a slow or unreachable
// endpoint cannot delay or drop events meant for the others.
type webhookDeliveryQueues struct {
	ctx     context.Context
	deliver func(context.Context, webhookDelivery)

	mu         sync.Mutex
	queues     map[string]*webhookQueue
	totalBytes int64
}

func newWebhookDeliveryQueues(ctx context.Context, deliver func(context.Context, webhookDelivery)) *webhookDeliveryQueues {
	return &webhookDeliveryQueues{ctx: ctx, deliver: deliver, queues: make(map[string]*webhookQueue)}
}

// enqueue never blocks. It reports false when the webhook's queue, or the
// memory set aside for all queues, is full.
func (q *webhookDeliveryQueues) enqueue(delivery webhookDelivery) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	queue, ok := q.queues[delivery.hookID]
	if !ok {
		queue = &webhookQueue{deliveries: make(chan webhookDelivery, webhookQueueDepth)}
		q.queues[delivery.hookID] = queue
		go q.run(delivery.hookID, queue)
	}
	size := delivery.size()
	if queue.bytes+size > webhookQueueMaxBytes || q.totalBytes+size > webhookQueuesMaxBytes {
		return false
	}
	select {
	case queue.deliveries <- delivery:
		queue.bytes += size
		q.totalBytes += size
		return true
	default:
		return false
	}
}

func (q *webhookDeliveryQueues) run(hookID string, queue *webhookQueue) {
	idle := time.NewTimer(webhookWorkerIdleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-q.ctx.Done():
			return
		case delivery := <-queue.deliveries:
			q.mu.Lock()
			queue.bytes -= delivery.size()
			q.totalBytes -= delivery.size()
			q.mu.Unlock()
			q.deliver(q.ctx, delivery)
			idle.Reset(webhookWorkerIdleTimeout)
		case <-idle.C:
			// Senders hold the lock while they queue, so an empty queue seen
			// under the lock stays empty until this worker is removed.
			q.mu.Lock()
			if len(queue.deliveries) == 0 {
				delete(q.queues, hookID)
				q.mu.Unlock()
				return
			}
			q.mu.Unlock()
			idle.Reset(webhookWorkerIdleTimeout)
		}
	}
}
