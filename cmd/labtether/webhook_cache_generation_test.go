package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/webhooks"
)

// Hold the first actual store snapshot until the mutation handler has returned.
// All writes, later reads and trigger tracking still use the real Memory owner.
type heldWebhookSnapshotStore struct {
	persistence.WebhookStore
	once     sync.Once
	observed chan struct{}
	release  chan struct{}
	marked   chan string
}

func (s *heldWebhookSnapshotStore) ListWebhooks(ctx context.Context) ([]webhooks.Webhook, error) {
	rows, err := s.WebhookStore.ListWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	hold := false
	s.once.Do(func() { hold = true })
	if hold {
		close(s.observed)
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return rows, nil
}

func (s *heldWebhookSnapshotStore) MarkWebhookTriggered(ctx context.Context, id string, at time.Time) error {
	err := s.WebhookStore.MarkWebhookTriggered(ctx, id, at)
	if err == nil {
		s.marked <- id
	}
	return err
}

func TestWebhookRelayDoesNotPublishInvalidatedSnapshot(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	for _, mutation := range []struct {
		name, method, body string
	}{
		{"disable", http.MethodPatch, `{"enabled":false}`},
		{"delete", http.MethodDelete, ""},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			s := newTestAPIServer(t)
			store := &heldWebhookSnapshotStore{
				WebhookStore: s.webhookStore, observed: make(chan struct{}),
				release: make(chan struct{}), marked: make(chan string, 8),
			}
			s.webhookStore = store
			s.webhookEventCh = make(chan webhookDispatchEvent, 4)
			var mu sync.Mutex
			var paths []string
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				w.WriteHeader(http.StatusNoContent)
			}))
			defer sink.Close()
			create := func(name, event string) string {
				body, err := json.Marshal(map[string]any{
					"name": name, "url": sink.URL + "/" + name, "events": []string{event},
				})
				if err != nil {
					t.Fatal(err)
				}
				return createWebhookForTest(t, s, string(body))
			}
			target := create("changed", "asset.online")
			control := create("control", "asset.offline")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			done := make(chan struct{})
			go func() {
				defer close(done)
				s.runWebhookRelay(ctx)
			}()
			var release sync.Once
			defer func() {
				release.Do(func() { close(store.release) })
				cancel()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("webhook relay did not stop")
				}
			}()

			s.enqueueWebhookEvent("asset.online", map[string]any{"asset_id": "before-change"}, time.Now())
			select {
			case <-store.observed:
			case <-ctx.Done():
				t.Fatal("relay did not reach the held store snapshot")
			}
			req := httptest.NewRequest(mutation.method, "/api/v2/webhooks/"+target, strings.NewReader(mutation.body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(contextWithPrincipal(ctx, "admin", "admin"))
			reply := httptest.NewRecorder()
			s.handleV2WebhookActions(reply, req)
			if reply.Code != http.StatusOK {
				t.Fatalf("mutation failed: %d %s", reply.Code, reply.Body.String())
			}
			release.Do(func() { close(store.release) })
			s.enqueueWebhookEvent("asset.online", map[string]any{"asset_id": "after-change"}, time.Now())
			// A distinct, still-enabled subscription is a positive completion barrier:
			// the sequential relay must finish both online events before this one.
			s.enqueueWebhookEvent("asset.offline", map[string]any{"asset_id": "barrier"}, time.Now())
			for {
				select {
				case id := <-store.marked:
					if id != control {
						continue
					}
				case <-ctx.Done():
					t.Fatal("unchanged subscription did not deliver after invalidation")
				}
				break
			}
			mu.Lock()
			received := append([]string(nil), paths...)
			mu.Unlock()
			if len(received) != 1 || received[0] != "/control" {
				t.Fatalf("disabled/deleted subscription received an event: %v", received)
			}
			stored, found, err := store.GetWebhook(ctx, target)
			if err != nil {
				t.Fatal(err)
			}
			if mutation.method == http.MethodDelete {
				if found {
					t.Fatal("deleted webhook returned")
				}
			} else if !found || stored.Enabled || stored.LastTriggeredAt != nil {
				t.Fatalf("disabled webhook changed or was triggered: %+v", stored)
			}
		})
	}
}
