package docker

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

const (
	eventReconnectMin     = 1 * time.Second
	eventReconnectMax     = 30 * time.Second
	eventDisconnectedPoll = 5 * time.Second
)

// runEventLoop listens for Docker daemon events and forwards them to the hub.
// Reconnects with exponential backoff if the event stream drops.
func (dc *DockerCollector) runEventLoop(ctx context.Context) {
	backoff := eventReconnectMin

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !dc.transportConnected() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(eventDisconnectedPoll):
				continue
			}
		}

		events := make(chan DockerEvent, 64)
		streamCtx, streamCancel := context.WithCancel(ctx)

		go func() {
			err := dc.client.streamEvents(streamCtx, events)
			if err != nil && ctx.Err() == nil {
				log.Printf("docker: event stream error: %v", err)
			}
			streamCancel()
		}()

		// Process events from the channel until the stream context is cancelled.
		for {
			select {
			case <-streamCtx.Done():
				goto reconnect
			case ev, ok := <-events:
				if !ok {
					goto reconnect
				}
				if !dc.transportConnected() {
					goto reconnect
				}
				backoff = eventReconnectMin // reset on successful event
				dc.queueDiscoveryForEvent(ev)
				dc.forwardEvent(ev)
			}
		}

	reconnect:
		streamCancel()
		if ctx.Err() != nil {
			return
		}
		log.Printf("docker: event stream disconnected, reconnecting in %v", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > eventReconnectMax {
			backoff = eventReconnectMax
		}
	}
}

func (dc *DockerCollector) forwardEvent(ev DockerEvent) {
	if !dc.transportConnected() {
		return
	}

	eventData := agentmgr.DockerEventData{
		HostID: dc.assetID,
		Type:   ev.Type,
		Action: ev.Action,
		Actor: agentmgr.DockerEventActor{
			ID:         ev.Actor.ID,
			Attributes: ev.Actor.Attributes,
		},
		Timestamp: ev.Time,
	}

	data, err := json.Marshal(eventData)
	if err != nil {
		log.Printf("docker: failed to marshal event: %v", err)
		return
	}

	if err := dc.transport.Send(agentmgr.Message{
		Type: agentmgr.MsgDockerEvents,
		Data: data,
	}); err != nil {
		log.Printf("docker: failed to send event: %v", err)
	}
}

func (dc *DockerCollector) queueDiscoveryForEvent(ev DockerEvent) {
	eventType := strings.ToLower(strings.TrimSpace(ev.Type))
	action := strings.ToLower(strings.TrimSpace(ev.Action))

	switch eventType {
	case "container":
		immediate := action == "start" || action == "stop" || action == "die" || action == "destroy" || action == "create" || action == "rename"
		dc.queueDiscoveryTrigger(false, immediate)
		dc.queueStatsTrigger()
	case "image", "network", "volume":
		// Non-container objects are updated on the full reconciliation path.
		dc.queueDiscoveryTrigger(true, false)
	default:
		// Unknown event types are still a useful hint that state may have changed.
		dc.queueDiscoveryTrigger(false, false)
	}
}
