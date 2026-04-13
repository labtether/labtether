package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebhookRelayDeliversBroadcastEventsAndMarksTriggered(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	s := newTestAPIServer(t)
	s.broadcaster = newEventBroadcaster()
	s.webhookEventCh = make(chan webhookDispatchEvent, 32)
	s.broadcaster.SetOnEvent(s.enqueueWebhookEvent)

	webhookID := createWebhookForTest(t, s, `{"name":"runtime","url":"http://127.0.0.1/hook","secret":"runtime-secret","events":["asset.online"]}`)

	type receivedRequest struct {
		body      map[string]any
		event     string
		timestamp string
		signature string
	}
	receivedCh := make(chan receivedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payloadBytes, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(payloadBytes, &payload)
		receivedCh <- receivedRequest{
			body:      payload,
			event:     r.Header.Get("X-Labtether-Event"),
			timestamp: r.Header.Get("X-Labtether-Timestamp"),
			signature: r.Header.Get("X-Labtether-Signature-256"),
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stored, ok, err := s.webhookStore.GetWebhook(context.Background(), webhookID)
	if err != nil || !ok {
		t.Fatalf("load stored webhook: ok=%v err=%v", ok, err)
	}
	stored.URL = srv.URL
	if err := s.webhookStore.UpdateWebhook(context.Background(), stored); err != nil {
		t.Fatalf("update webhook URL: %v", err)
	}
	s.invalidateWebhookCache()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.runWebhookRelay(ctx)

	s.broadcaster.Broadcast("asset.online", map[string]any{"asset_id": "asset-1"})

	var received receivedRequest
	select {
	case received = <-receivedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
	}

	if received.event != "asset.online" {
		t.Fatalf("expected event header asset.online, got %q", received.event)
	}
	if received.timestamp == "" {
		t.Fatal("expected timestamp header to be set")
	}
	if received.signature == "" {
		t.Fatal("expected signature header to be set")
	}
	if !strings.HasPrefix(received.signature, "sha256=") {
		t.Fatalf("expected sha256 signature prefix, got %q", received.signature)
	}
	if received.body["type"] != "asset.online" {
		t.Fatalf("expected payload type asset.online, got %v", received.body["type"])
	}

	waitForWebhookTrigger(t, s, webhookID)
}

func TestWebhookRelaySkipsNonMatchingEvents(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	s := newTestAPIServer(t)
	s.broadcaster = newEventBroadcaster()
	s.webhookEventCh = make(chan webhookDispatchEvent, 32)
	s.broadcaster.SetOnEvent(s.enqueueWebhookEvent)

	webhookID := createWebhookForTest(t, s, `{"name":"runtime","url":"http://127.0.0.1/hook","events":["asset.offline"]}`)

	requested := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stored, ok, err := s.webhookStore.GetWebhook(context.Background(), webhookID)
	if err != nil || !ok {
		t.Fatalf("load stored webhook: ok=%v err=%v", ok, err)
	}
	stored.URL = srv.URL
	if err := s.webhookStore.UpdateWebhook(context.Background(), stored); err != nil {
		t.Fatalf("update webhook URL: %v", err)
	}
	s.invalidateWebhookCache()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.runWebhookRelay(ctx)

	s.broadcaster.Broadcast("asset.online", map[string]any{"asset_id": "asset-1"})

	select {
	case <-requested:
		t.Fatal("did not expect a webhook request for non-matching event")
	case <-time.After(300 * time.Millisecond):
	}

	stored, ok, err = s.webhookStore.GetWebhook(context.Background(), webhookID)
	if err != nil || !ok {
		t.Fatalf("reload stored webhook: ok=%v err=%v", ok, err)
	}
	if stored.LastTriggeredAt != nil {
		t.Fatalf("expected LastTriggeredAt to remain nil, got %v", stored.LastTriggeredAt)
	}
}

func waitForWebhookTrigger(t *testing.T, s *apiServer, webhookID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		wh, ok, err := s.webhookStore.GetWebhook(context.Background(), webhookID)
		if err == nil && ok && wh.LastTriggeredAt != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	wh, _, _ := s.webhookStore.GetWebhook(context.Background(), webhookID)
	t.Fatalf("expected LastTriggeredAt to be set, got %#v", wh.LastTriggeredAt)
}
