package notifications

// Risks covered: the Hub must send the exact request the relay accepts (or
// every push fails); only the relay's explicit "gone" answers may delete a
// registration (a wrong URL answering 404 must not wipe every device); a
// relay id is a case-sensitive bearer secret that must never be lowercased
// or reach logs and errors.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	relayIDGone    = "GoneRegistration_abcdefghijklmnopqrstuvwxyz"
	relayIDUnknown = "UnknownRegistration-ABCDEFGHIJKLMNOPQRSTUVW"
	relayIDOK      = "OkRegistration_0123456789-ABCDEFGHIJKLMNOPQ"
)

type relayCapture struct {
	mu       sync.Mutex
	requests []capturedRelayRequest
}

type capturedRelayRequest struct {
	method, path, contentType, authorization string
	body                                     map[string]any
}

// newRelayServer answers like services/push-relay: per registration id.
func newRelayServer(t *testing.T, answers map[string]func(http.ResponseWriter)) (*httptest.Server, *relayCapture) {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	capture := &relayCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		capture.mu.Lock()
		capture.requests = append(capture.requests, capturedRelayRequest{
			method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type"),
			authorization: r.Header.Get("Authorization"), body: body,
		})
		capture.mu.Unlock()
		id, _ := body["registration_id"].(string)
		if answer, ok := answers[id]; ok {
			answer(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"apns_id":null}`))
	}))
	t.Cleanup(server.Close)
	return server, capture
}

func relayAnswer(status int, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buffer)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &buffer
}

func TestRelayAlertRequestShapeKeepsIDCaseAndCarriesNoKey(t *testing.T) {
	server, capture := newRelayServer(t, nil)
	adapter := &APNsAdapter{Relay: NewAPNsRelayClient(server.URL)}
	logs := captureLogs(t)
	err := adapter.Send(context.Background(), map[string]any{
		"transport": "relay", "device_tokens": []string{relayIDOK},
	}, map[string]any{
		"title": "Disk full", "text": "nas01 at 97%", "alert_id": "al_42",
		"apns_priority": 5, "apns_expiration_unix": int64(1_800_000_000),
	})
	if err != nil {
		t.Fatalf("relay send: %v", err)
	}
	if len(capture.requests) != 1 {
		t.Fatalf("relay requests = %d, want 1", len(capture.requests))
	}
	got := capture.requests[0]
	if got.method != http.MethodPost || got.path != "/v1/push" || got.contentType != "application/json" || got.authorization != "" {
		t.Fatalf("unexpected relay request line/headers: %+v", got)
	}
	want := map[string]any{
		"registration_id": relayIDOK, "push_type": "alert", "priority": float64(5),
		"expiration": float64(1_800_000_000), "collapse_id": "al_42",
	}
	for key, value := range want {
		if got.body[key] != value {
			t.Fatalf("relay body %s = %#v, want %#v (body %v)", key, got.body[key], value, got.body)
		}
	}
	if len(got.body) != 6 {
		t.Fatalf("relay refuses unknown fields; body has %d keys: %v", len(got.body), got.body)
	}
	payload, _ := got.body["payload"].(map[string]any)
	aps, _ := payload["aps"].(map[string]any)
	alert, _ := aps["alert"].(map[string]any)
	if alert["title"] != "Disk full" || alert["body"] != "nas01 at 97%" {
		t.Fatalf("relay payload lost the APNs alert: %v", payload)
	}
	if strings.Contains(logs.String(), relayIDOK) {
		t.Fatal("relay registration id reached the log")
	}
}

func TestRelayOnlyExplicitGoneAnswersTriggerCleanup(t *testing.T) {
	server, _ := newRelayServer(t, map[string]func(http.ResponseWriter){
		relayIDGone:    relayAnswer(http.StatusGone, `{"error":"unregistered"}`),
		relayIDUnknown: relayAnswer(http.StatusNotFound, `{"error":"unknown_registration"}`),
	})
	adapter := &APNsAdapter{Relay: NewAPNsRelayClient(server.URL)}
	var mu sync.Mutex
	var cleaned []string
	config := map[string]any{"transport": "relay", "device_tokens": []string{relayIDGone, relayIDUnknown, relayIDOK}}
	SetAPNsInvalidDeviceTokenHandler(config, func(token string) error {
		mu.Lock()
		defer mu.Unlock()
		cleaned = append(cleaned, token)
		return nil
	})
	err := adapter.Send(context.Background(), config, map[string]any{"title": "t", "text": "x"})
	indices, indexed := APNsFailedDeliveryIndices(err)
	if !indexed || len(indices) != 2 {
		t.Fatalf("want two indexed failures, got %v (%v)", indices, err)
	}
	if len(cleaned) != 2 || !containsString(cleaned, relayIDGone) || !containsString(cleaned, relayIDUnknown) {
		t.Fatalf("cleanup = %v, want exactly the gone and unknown ids", cleaned)
	}
	for _, id := range []string{relayIDGone, relayIDUnknown, relayIDOK} {
		if strings.Contains(err.Error(), id) {
			t.Fatal("relay registration id reached a delivery error")
		}
	}

	for _, answer := range []func(http.ResponseWriter){
		relayAnswer(http.StatusNotFound, `{"error":"not_found"}`),
		relayAnswer(http.StatusNotFound, `<html>wrong host</html>`),
		relayAnswer(http.StatusTooManyRequests, `{"error":"rate_limited"}`),
		relayAnswer(http.StatusServiceUnavailable, `{"error":"not_configured"}`),
		relayAnswer(http.StatusBadGateway, `{"error":"apns_error","reason":"BadDeviceToken"}`),
	} {
		server, _ := newRelayServer(t, map[string]func(http.ResponseWriter){relayIDOK: answer})
		relayErr := NewAPNsRelayClient(server.URL).push(context.Background(), apnsRelayPush{
			RegistrationID: relayIDOK, PushType: "alert", Priority: 10, Payload: json.RawMessage(`{"aps":{}}`),
		})
		if relayErr == nil || isPermanentAPNsTokenRejection(relayErr) {
			t.Fatalf("answer must be a retryable, non-permanent error, got %v", relayErr)
		}
	}
}

func TestRelayLiveActivityKeepsIDCaseAndActivityCollapseID(t *testing.T) {
	server, capture := newRelayServer(t, nil)
	adapter := &APNsAdapter{Relay: NewAPNsRelayClient(server.URL)}
	now := time.Now().UTC()
	push := LiveActivityPush{
		DeviceToken: " " + relayIDOK + " ", BundleID: OfficialAppBundleID, ActivityID: "activity-1",
		Event: "update", Priority: 5, Timestamp: now, ExpiresAt: now.Add(time.Hour),
		State: LiveActivityContentState{Title: "Incident", Status: "open", Severity: "high", StartedAt: now, UpdatedAt: now},
	}
	if err := adapter.SendLiveActivity(context.Background(), map[string]any{"transport": "relay"}, push); err != nil {
		t.Fatalf("relay live activity: %v", err)
	}
	body := capture.requests[0].body
	if body["registration_id"] != relayIDOK || body["push_type"] != "liveactivity" || body["collapse_id"] != "activity-1" || body["priority"] != float64(5) {
		t.Fatalf("unexpected relay live activity body: %v", body)
	}
	push.BundleID = "com.example.selfbuilt"
	if err := adapter.SendLiveActivity(context.Background(), map[string]any{"transport": "relay"}, push); err == nil {
		t.Fatal("relay must refuse a bundle other than the official app")
	}
}

func TestParsePushRelayURL(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	for _, disabled := range []string{"", " off ", "OFF", "none"} {
		if got, err := ParsePushRelayURL(disabled); got != "" || err != nil {
			t.Fatalf("ParsePushRelayURL(%q) = %q, %v; want disabled", disabled, got, err)
		}
	}
	if got, err := ParsePushRelayURL("https://push.labtether.com/"); err != nil || got != DefaultPushRelayURL {
		t.Fatalf("default relay URL = %q, %v", got, err)
	}
	for _, invalid := range []string{"push.labtether.com", "http://push.labtether.com", "https://user:pw@push.labtether.com", "https://push.labtether.com/?x=1"} {
		if _, err := ParsePushRelayURL(invalid); err == nil {
			t.Fatalf("ParsePushRelayURL(%q) accepted an unsafe URL", invalid)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
