package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNtfyAdapterSend(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	var gotPath string
	var gotTitle string
	var gotPriority string
	var gotAuth string
	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTitle = r.Header.Get("Title")
		gotPriority = r.Header.Get("Priority")
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := &NtfyAdapter{}
	err := adapter.Send(context.Background(), map[string]any{
		"server_url": server.URL,
		"topic":      "ops-alerts",
		"priority":   4,
		"token":      "abc123",
	}, map[string]any{
		"title": "Disk Alert",
		"text":  "Disk usage crossed threshold",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if gotPath != "/ops-alerts" {
		t.Fatalf("path = %q, want /ops-alerts", gotPath)
	}
	if gotTitle != "Disk Alert" {
		t.Fatalf("title header = %q, want Disk Alert", gotTitle)
	}
	if gotPriority != "4" {
		t.Fatalf("priority header = %q, want 4", gotPriority)
	}
	if gotAuth != "Bearer abc123" {
		t.Fatalf("auth header = %q, want Bearer abc123", gotAuth)
	}
	if gotBody != "Disk usage crossed threshold" {
		t.Fatalf("body = %q, want payload text", gotBody)
	}
}

func TestNtfyAdapterSendMissingConfig(t *testing.T) {
	adapter := &NtfyAdapter{}
	if err := adapter.Send(context.Background(), map[string]any{}, map[string]any{"text": "hello"}); err == nil {
		t.Fatal("expected error for missing server_url/topic")
	}
}

func TestGotifyAdapterSend(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	var gotPath string
	var gotKey string
	var gotPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-Gotify-Key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := &GotifyAdapter{}
	err := adapter.Send(context.Background(), map[string]any{
		"server_url": server.URL,
		"app_token":  "gotify-token",
		"priority":   "5",
	}, map[string]any{
		"title": "CPU Alert",
		"text":  "High CPU usage detected",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if gotPath != "/message" {
		t.Fatalf("path = %q, want /message", gotPath)
	}
	if gotKey != "gotify-token" {
		t.Fatalf("X-Gotify-Key = %q, want gotify-token", gotKey)
	}
	if gotPayload["title"] != "CPU Alert" {
		t.Fatalf("title = %v, want CPU Alert", gotPayload["title"])
	}
	if gotPayload["message"] != "High CPU usage detected" {
		t.Fatalf("message = %v, want High CPU usage detected", gotPayload["message"])
	}
}

func TestGotifyAdapterSendMissingToken(t *testing.T) {
	adapter := &GotifyAdapter{}
	err := adapter.Send(context.Background(), map[string]any{"server_url": "https://gotify.local"}, map[string]any{"text": "hello"})
	if err == nil || !strings.Contains(err.Error(), "app_token") {
		t.Fatalf("expected missing app_token error, got %v", err)
	}
}

func TestNormalizeChannelTypeSupportsNtfyAndGotify(t *testing.T) {
	if got := NormalizeChannelType(" NTFY "); got != ChannelTypeNtfy {
		t.Fatalf("NormalizeChannelType(ntfy) = %q", got)
	}
	if got := NormalizeChannelType("gotify"); got != ChannelTypeGotify {
		t.Fatalf("NormalizeChannelType(gotify) = %q", got)
	}
}
