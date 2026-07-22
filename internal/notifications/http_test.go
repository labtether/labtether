package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSlackAdapterPostsExpectedLocalWebhookPayload(t *testing.T) {
	allowLoopbackHTTP(t)

	var method string
	var contentType string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &payload)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	err := (&SlackAdapter{}).Send(context.Background(), map[string]any{
		"webhook_url": server.URL,
	}, map[string]any{
		"title": "Critical alert",
		"text":  "Disk usage exceeded threshold",
	})
	if err != nil {
		t.Fatalf("send Slack webhook to local sink: %v", err)
	}
	if method != http.MethodPost || contentType != "application/json" {
		t.Fatalf("Slack request = %s %q, want POST application/json", method, contentType)
	}
	if payload["text"] != "Critical alert Disk usage exceeded threshold" {
		t.Fatalf("Slack fallback text = %v", payload["text"])
	}
	blocks, ok := payload["blocks"].([]any)
	if !ok || len(blocks) != 2 {
		t.Fatalf("Slack blocks = %#v, want header and section", payload["blocks"])
	}
}

func TestNotificationHTTPAdaptersRejectNon2xxResponses(t *testing.T) {
	allowLoopbackHTTP(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMultipleChoices)
	}))
	t.Cleanup(server.Close)

	tests := []struct {
		name    string
		adapter Adapter
		config  map[string]any
		payload map[string]any
	}{
		{name: "webhook", adapter: &WebhookAdapter{}, config: map[string]any{"url": server.URL}, payload: map[string]any{"text": "test"}},
		{name: "slack", adapter: &SlackAdapter{}, config: map[string]any{"webhook_url": server.URL}, payload: map[string]any{"text": "test"}},
		{name: "ntfy", adapter: &NtfyAdapter{}, config: map[string]any{"server_url": server.URL, "topic": "ops"}, payload: map[string]any{"text": "test"}},
		{name: "gotify", adapter: &GotifyAdapter{}, config: map[string]any{"server_url": server.URL, "app_token": "secret"}, payload: map[string]any{"text": "test"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.adapter.Send(context.Background(), test.config, test.payload)
			if err == nil || !strings.Contains(err.Error(), "status 300") {
				t.Fatalf("expected status 300 error, got %v", err)
			}
		})
	}
}

func TestWebhookAdapterDoesNotForwardCustomHeadersAcrossOriginRedirect(t *testing.T) {
	allowLoopbackHTTP(t)

	var targetCalls atomic.Int64
	var leakedHeader atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		leakedHeader.Store(r.Header.Get("X-Api-Key"))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(target.Close)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(origin.Close)

	err := (&WebhookAdapter{}).Send(context.Background(), map[string]any{
		"url": origin.URL,
		"headers": map[string]any{
			"X-Api-Key": "must-not-leak",
		},
	}, map[string]any{"text": "test"})
	if err == nil || !strings.Contains(err.Error(), "different origin") {
		t.Fatalf("expected cross-origin redirect error, got %v", err)
	}
	if calls := targetCalls.Load(); calls != 0 {
		t.Fatalf("redirect target received %d calls; custom headers could have leaked", calls)
	}
	if got := leakedHeader.Load(); got != nil {
		t.Fatalf("redirect target received secret header %q", got)
	}
}

func TestGotifyAdapterDoesNotForwardTokenAcrossOriginRedirect(t *testing.T) {
	allowLoopbackHTTP(t)

	var targetCalls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		if got := r.Header.Get("X-Gotify-Key"); got != "" {
			t.Errorf("redirect target received Gotify token %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(target.Close)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL+"/message")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	t.Cleanup(origin.Close)

	err := (&GotifyAdapter{}).Send(context.Background(), map[string]any{
		"server_url": origin.URL,
		"app_token":  "must-not-leak",
	}, map[string]any{"text": "test"})
	if err == nil || !strings.Contains(err.Error(), "different origin") {
		t.Fatalf("expected cross-origin redirect error, got %v", err)
	}
	if calls := targetCalls.Load(); calls != 0 {
		t.Fatalf("redirect target received %d calls; token could have leaked", calls)
	}
}
