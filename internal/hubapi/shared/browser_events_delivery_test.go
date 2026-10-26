package shared

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type countedEvent struct{ encoded *int }

func (e countedEvent) MarshalJSON() ([]byte, error) {
	*e.encoded++
	return []byte(`{"ok":true}`), nil
}

func TestBroadcastWithoutBrowsersStillInvokesCallbacks(t *testing.T) {
	eb := NewEventBroadcaster()
	encoded, broadcasts, events := 0, 0, 0
	eb.SetOnBroadcast(func() { broadcasts++ })
	eb.SetOnEvent(func(kind string, data any, at time.Time) {
		events++
		if kind != "event" || at.IsZero() || data == nil {
			t.Error("callback lost event envelope")
		}
	})
	eb.Broadcast("event", countedEvent{encoded: &encoded})
	if encoded != 0 || broadcasts != 1 || events != 1 {
		t.Fatalf("encoded=%d broadcasts=%d events=%d", encoded, broadcasts, events)
	}
}

func TestBroadcastPreservesQueuedFramesAndEvictsSlowClients(t *testing.T) {
	eb := NewEventBroadcaster()
	fast := NewBrowserClientForTesting(nil, make(chan []byte, 3))
	slow := NewBrowserClientForTesting(nil, make(chan []byte, 1))
	eb.InjectClientForTesting(fast)
	eb.InjectClientForTesting(slow)
	data := map[string]any{"sequence": 1}
	eb.Broadcast("event", data)
	data["sequence"] = 2
	eb.Broadcast("event", data)
	data["sequence"] = 3
	eb.Broadcast("event", data)
	if eb.Count() != 1 {
		t.Fatalf("clients=%d, want only fast client", eb.Count())
	}
	for want := 1; want <= 3; want++ {
		var frame struct {
			Data struct{ Sequence int } `json:"data"`
		}
		if err := json.Unmarshal(<-fast.Outgoing, &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Data.Sequence != want {
			t.Fatalf("sequence=%d, want %d", frame.Data.Sequence, want)
		}
	}
	<-slow.Outgoing
	if _, open := <-slow.Outgoing; open {
		t.Fatal("slow client's queue was not closed")
	}
}

func TestBroadcastConcurrentClientClose(t *testing.T) {
	eb := NewEventBroadcaster()
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for i := 0; i < 100; i++ {
				client := NewBrowserClientForTesting(nil, make(chan []byte, 1))
				eb.InjectClientForTesting(client)
				eb.Broadcast("event", i)
				eb.Unregister(client)
			}
		})
	}
	workers.Wait()
	if eb.Count() != 0 {
		t.Fatalf("clients remain: %d", eb.Count())
	}
}
