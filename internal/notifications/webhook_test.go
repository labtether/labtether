package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// allowLoopbackHTTP opts the test process into using plain HTTP against
// 127.0.0.1 — the security runtime blocks this by default.
func allowLoopbackHTTP(t *testing.T) {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
}

// ── Type() ───────────────────────────────────────────────────────────────────

func TestWebhookAdapter_Type(t *testing.T) {
	a := &WebhookAdapter{}
	if got := a.Type(); got != ChannelTypeWebhook {
		t.Errorf("Type() = %q, want %q", got, ChannelTypeWebhook)
	}
}

// ── Send — happy path ─────────────────────────────────────────────────────────

func TestWebhookAdapter_Send_PostsJSONBody(t *testing.T) {
	allowLoopbackHTTP(t)

	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := &WebhookAdapter{}
	payload := map[string]any{"event": "alert.fired", "severity": "critical"}
	err := a.Send(context.Background(), map[string]any{"url": srv.URL + "/hook"}, payload)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(received, &got); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if got["event"] != "alert.fired" {
		t.Errorf("event = %v, want %q", got["event"], "alert.fired")
	}
}

func TestWebhookAdapter_Send_SetsContentTypeJSON(t *testing.T) {
	allowLoopbackHTTP(t)

	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := &WebhookAdapter{}
	_ = a.Send(context.Background(), map[string]any{"url": srv.URL}, map[string]any{"k": "v"})
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}
}

func TestWebhookAdapter_Send_ForwardsCustomHeaders(t *testing.T) {
	allowLoopbackHTTP(t)

	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Api-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := &WebhookAdapter{}
	cfg := map[string]any{
		"url": srv.URL,
		"headers": map[string]any{
			"X-Api-Key": "tok3n-abc",
		},
	}
	if err := a.Send(context.Background(), cfg, map[string]any{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotHeader != "tok3n-abc" {
		t.Errorf("X-Api-Key = %q, want %q", gotHeader, "tok3n-abc")
	}
}

func TestWebhookAdapter_Send_SetsSignatureHeadersWhenConfigured(t *testing.T) {
	allowLoopbackHTTP(t)

	var eventHeader string
	var timestampHeader string
	var signatureHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventHeader = r.Header.Get("X-Labtether-Event")
		timestampHeader = r.Header.Get("X-Labtether-Timestamp")
		signatureHeader = r.Header.Get("X-Labtether-Signature-256")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := &WebhookAdapter{}
	cfg := map[string]any{
		"url":        srv.URL,
		"secret":     "test-secret",
		"event_type": "asset.online",
		"timestamp":  "2026-04-12T01:02:03Z",
	}
	payload := map[string]any{"type": "asset.online", "data": map[string]any{"asset_id": "asset-1"}}
	if err := a.Send(context.Background(), cfg, payload); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if eventHeader != "asset.online" {
		t.Fatalf("expected event header, got %q", eventHeader)
	}
	if timestampHeader != "2026-04-12T01:02:03Z" {
		t.Fatalf("expected timestamp header, got %q", timestampHeader)
	}
	expectedSignatureBody, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	expectedSignature := webhookSignature("test-secret", "2026-04-12T01:02:03Z", expectedSignatureBody)
	if signatureHeader != expectedSignature {
		t.Fatalf("expected signature %q, got %q", expectedSignature, signatureHeader)
	}
}

func TestWebhookAdapter_Send_UsesPostMethod(t *testing.T) {
	allowLoopbackHTTP(t)

	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := &WebhookAdapter{}
	_ = a.Send(context.Background(), map[string]any{"url": srv.URL}, map[string]any{})
	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
}

// ── Send — error conditions ───────────────────────────────────────────────────

func TestWebhookAdapter_Send_MissingURL(t *testing.T) {
	a := &WebhookAdapter{}
	err := a.Send(context.Background(), map[string]any{}, map[string]any{"k": "v"})
	if err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
}

func TestWebhookAdapter_Send_EmptyURL(t *testing.T) {
	a := &WebhookAdapter{}
	err := a.Send(context.Background(), map[string]any{"url": "   "}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty url, got nil")
	}
}

func TestWebhookAdapter_Send_NonStringURL(t *testing.T) {
	a := &WebhookAdapter{}
	err := a.Send(context.Background(), map[string]any{"url": 12345}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for non-string url, got nil")
	}
}

func TestWebhookAdapter_Send_4xxResponseIsError(t *testing.T) {
	allowLoopbackHTTP(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	a := &WebhookAdapter{}
	err := a.Send(context.Background(), map[string]any{"url": srv.URL}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for 4xx response, got nil")
	}
}

func TestWebhookAdapter_Send_5xxResponseIsError(t *testing.T) {
	allowLoopbackHTTP(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := &WebhookAdapter{}
	err := a.Send(context.Background(), map[string]any{"url": srv.URL}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for 5xx response, got nil")
	}
}

func TestWebhookAdapter_Send_2xxResponseIsSuccess(t *testing.T) {
	allowLoopbackHTTP(t)

	for _, code := range []int{200, 201, 204} {
		code := code
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer srv.Close()

			a := &WebhookAdapter{}
			if err := a.Send(context.Background(), map[string]any{"url": srv.URL}, map[string]any{}); err != nil {
				t.Errorf("status %d: unexpected error: %v", code, err)
			}
		})
	}
}

// TestWebhookAdapter_Send_ContextCancelled verifies that a cancelled context
// causes Send to return an error without completing the request.
func TestWebhookAdapter_Send_ContextCancelled(t *testing.T) {
	allowLoopbackHTTP(t)

	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	a := &WebhookAdapter{}
	err := a.Send(ctx, map[string]any{"url": srv.URL}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestWebhookAdapter_Send_NonJSONHeadersFieldIgnored verifies that a headers
// value with a wrong type is silently skipped rather than causing a panic.
func TestWebhookAdapter_Send_NonStringHeaderValuesIgnored(t *testing.T) {
	allowLoopbackHTTP(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := &WebhookAdapter{}
	cfg := map[string]any{
		"url": srv.URL,
		"headers": map[string]any{
			"X-Numeric": 42, // non-string — should be silently skipped
		},
	}
	// Must not panic; must succeed because the server returns 200.
	if err := a.Send(context.Background(), cfg, map[string]any{}); err != nil {
		t.Fatalf("Send with non-string header value: %v", err)
	}
}
