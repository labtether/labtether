package notifications

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAPNsAdapter_Type(t *testing.T) {
	a := &APNsAdapter{}
	if a.Type() != "apns" {
		t.Fatalf("expected type 'apns', got %q", a.Type())
	}
}

func TestAPNsAdapter_Send_MissingConfig(t *testing.T) {
	a := &APNsAdapter{}
	err := a.Send(context.Background(), map[string]any{}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestAPNsAdapter_Send_PartialConfig(t *testing.T) {
	a := &APNsAdapter{}
	// Provide some but not all required fields.
	err := a.Send(context.Background(), map[string]any{
		"auth_key_path": "/some/path.p8",
		"key_id":        "ABC123",
	}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for partial config")
	}
}

func TestAPNsAdapter_Send_NoDeviceTokens(t *testing.T) {
	a := &APNsAdapter{}
	// All config present but no device tokens — should succeed silently.
	err := a.Send(context.Background(), map[string]any{
		"auth_key_path": "/some/path.p8",
		"key_id":        "ABC123DEF4",
		"team_id":       "TEAM123456",
		"bundle_id":     "com.labtether.mobile",
	}, map[string]any{})
	if err != nil {
		t.Fatalf("expected nil error when no device tokens, got: %v", err)
	}
}

func TestAPNsAdapterSendUsesBoundedConcurrentFanout(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	adapter := &APNsAdapter{authKey: key, keyPath: "cached-test-key.p8"}

	originalClient := sharedAPNsHTTPClient
	t.Cleanup(func() { sharedAPNsHTTPClient = originalClient })
	var inFlight atomic.Int32
	var maximumInFlight atomic.Int32
	sharedAPNsHTTPClient = &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			current := inFlight.Add(1)
			for {
				previous := maximumInFlight.Load()
				if current <= previous || maximumInFlight.CompareAndSwap(previous, current) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			inFlight.Add(-1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    request,
			}, nil
		}),
	}

	tokens := make([]string, 24)
	for index := range tokens {
		tokens[index] = fmt.Sprintf("token-%d", index)
	}
	err = adapter.Send(context.Background(), map[string]any{
		"auth_key_path": "cached-test-key.p8",
		"key_id":        "KEYID00001",
		"team_id":       "TEAMID0001",
		"bundle_id":     "com.labtether.mobile",
		"device_tokens": tokens,
	}, map[string]any{"title": "Test", "text": "Concurrent delivery"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := maximumInFlight.Load(); got <= 1 || got > apnsMaxConcurrentDeliveries {
		t.Fatalf("maximum concurrent deliveries = %d, want 2...%d", got, apnsMaxConcurrentDeliveries)
	}
}

func TestAPNsAdapterSendReportsOnlyFailedDeliveryIndices(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	adapter := &APNsAdapter{authKey: key, keyPath: "cached-test-key.p8"}

	originalClient := sharedAPNsHTTPClient
	t.Cleanup(func() { sharedAPNsHTTPClient = originalClient })
	sharedAPNsHTTPClient = &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			status := http.StatusOK
			body := ""
			if strings.HasSuffix(request.URL.Path, "/token-b") {
				status = http.StatusServiceUnavailable
				body = `{"reason":"ServiceUnavailable"}`
			}
			return &http.Response{
				StatusCode: status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    request,
			}, nil
		}),
	}

	err = adapter.Send(context.Background(), map[string]any{
		"auth_key_path": "cached-test-key.p8",
		"key_id":        "KEYID00001",
		"team_id":       "TEAMID0001",
		"bundle_id":     "com.labtether.mobile",
		"device_tokens": []string{"token-a", "token-b", "token-c"},
	}, map[string]any{"title": "Test", "text": "Partial delivery"})
	if err == nil {
		t.Fatal("expected one APNs delivery failure")
	}
	indices, ok := APNsFailedDeliveryIndices(err)
	if !ok {
		t.Fatalf("error did not retain positional APNs outcomes: %T %v", err, err)
	}
	if len(indices) != 1 || indices[0] != 1 {
		t.Fatalf("failed indices = %v, want [1]", indices)
	}
	for _, token := range []string{"token-a", "token-b", "token-c"} {
		if strings.Contains(err.Error(), token) {
			t.Fatalf("aggregate error exposed device token %q: %v", token, err)
		}
	}
}

func TestExtractDeviceTokens_StringSlice(t *testing.T) {
	tokens := extractDeviceTokens(map[string]any{
		"device_tokens": []string{"abc", "def"},
	})
	if len(tokens) != 2 || tokens[0] != "abc" || tokens[1] != "def" {
		t.Fatalf("unexpected tokens: %v", tokens)
	}
}

func TestExtractDeviceTokens_AnySlice(t *testing.T) {
	// JSON-decoded arrays arrive as []any.
	tokens := extractDeviceTokens(map[string]any{
		"device_tokens": []any{"token1", "token2", "", "token3"},
	})
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(tokens), tokens)
	}
}

func TestExtractDeviceTokens_Missing(t *testing.T) {
	tokens := extractDeviceTokens(map[string]any{})
	if tokens != nil {
		t.Fatalf("expected nil tokens, got %v", tokens)
	}
}

func TestBuildAPNsPayload(t *testing.T) {
	body, err := buildAPNsPayload(map[string]any{
		"title":         "Test Alert",
		"text":          "Something happened",
		"alert_id":      "alert-123",
		"apns_category": "LT_ALERT_ACTIONS",
		"deep_link":     "labtether://alerts/alert-123",
		"severity":      "critical",
		"event":         "alert.firing",
		"rule_id":       "rule-123",
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	aps, ok := result["aps"].(map[string]any)
	if !ok {
		t.Fatal("missing aps key")
	}

	alert, ok := aps["alert"].(map[string]any)
	if !ok {
		t.Fatal("missing aps.alert key")
	}
	if alert["title"] != "Test Alert" {
		t.Fatalf("unexpected title: %v", alert["title"])
	}
	if alert["body"] != "Something happened" {
		t.Fatalf("unexpected body: %v", alert["body"])
	}
	if aps["sound"] != "default" {
		t.Fatalf("unexpected sound: %v", aps["sound"])
	}
	if aps["category"] != "LT_ALERT_ACTIONS" {
		t.Fatalf("unexpected category: %v", aps["category"])
	}
	if result["alert_id"] != "alert-123" {
		t.Fatalf("unexpected alert_id: %v", result["alert_id"])
	}
	if result["deep_link"] != "labtether://alerts/alert-123" {
		t.Fatalf("unexpected deep_link: %v", result["deep_link"])
	}
	if result["severity"] != "critical" {
		t.Fatalf("unexpected severity: %v", result["severity"])
	}
	if result["event"] != "alert.firing" {
		t.Fatalf("unexpected event: %v", result["event"])
	}
	if result["rule_id"] != "rule-123" {
		t.Fatalf("unexpected rule_id: %v", result["rule_id"])
	}
}

func TestBuildAPNsPayload_DefaultTitle(t *testing.T) {
	body, err := buildAPNsPayload(map[string]any{
		"text": "body only",
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	aps := result["aps"].(map[string]any)
	alert := aps["alert"].(map[string]any)
	if alert["title"] != "LabTether Alert" {
		t.Fatalf("expected default title, got %v", alert["title"])
	}
}

func TestBuildAPNsPayloadBoundsOperatorControlledText(t *testing.T) {
	body, err := buildAPNsPayload(map[string]any{
		"title":     strings.Repeat("🧪", 100),
		"text":      strings.Repeat("payload", 1_000),
		"deep_link": strings.Repeat("x", 2_000),
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if len(body) > 4*1024 {
		t.Fatalf("payload is %d bytes, want <= 4096", len(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	aps := result["aps"].(map[string]any)
	alert := aps["alert"].(map[string]any)
	if len(alert["title"].(string)) > 160 {
		t.Fatalf("title was not byte-bounded: %d", len(alert["title"].(string)))
	}
	if len(alert["body"].(string)) > 1_024 {
		t.Fatalf("body was not byte-bounded: %d", len(alert["body"].(string)))
	}
	if len(result["deep_link"].(string)) > 512 {
		t.Fatalf("deep link was not byte-bounded: %d", len(result["deep_link"].(string)))
	}
}

func TestBuildAPNsPayload_FallsBackToAlertInstanceID(t *testing.T) {
	body, err := buildAPNsPayload(map[string]any{
		"title":             "Fallback",
		"text":              "Fallback body",
		"alert_instance_id": "instance-abc",
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if result["alert_id"] != "instance-abc" {
		t.Fatalf("expected alert_id fallback from alert_instance_id, got %v", result["alert_id"])
	}
	aps := result["aps"].(map[string]any)
	if _, present := aps["category"]; present {
		t.Fatalf("non-actionable alert payload unexpectedly received a category: %v", aps["category"])
	}
}

func TestBuildAPNsPayloadIncludesBoundedIncidentMetadata(t *testing.T) {
	body, err := buildAPNsPayload(map[string]any{
		"title":         "Database unavailable",
		"text":          "Primary storage is unreachable.",
		"incident_id":   "incident-123",
		"apns_category": "LT_INCIDENT_ACTIONS",
		"deep_link":     "labtether://incidents/incident-123",
		"severity":      "critical",
		"status":        "investigating",
		"summary":       strings.Repeat("summary", 500),
		"event":         "incident.status_changed",
	})
	if err != nil {
		t.Fatalf("build incident payload: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal incident payload: %v", err)
	}
	aps := result["aps"].(map[string]any)
	if aps["category"] != "LT_INCIDENT_ACTIONS" {
		t.Fatalf("incident category = %v", aps["category"])
	}
	for key, want := range map[string]string{
		"title":       "Database unavailable",
		"incident_id": "incident-123",
		"deep_link":   "labtether://incidents/incident-123",
		"severity":    "critical",
		"status":      "investigating",
		"event":       "incident.status_changed",
	} {
		if result[key] != want {
			t.Fatalf("incident %s = %v, want %q", key, result[key], want)
		}
	}
	if summary, _ := result["summary"].(string); len(summary) > 1_024 {
		t.Fatalf("incident summary bytes = %d, want <= 1024", len(summary))
	}
	if len(body) > 4*1024 {
		t.Fatalf("incident payload is %d bytes, want <= 4096", len(body))
	}
}

func TestPermanentAPNsTokenRejectionsAreClassifiedWithoutExposingToken(t *testing.T) {
	for _, reason := range []string{"BadDeviceToken", "DeviceTokenNotForTopic", "Unregistered"} {
		status := 400
		if reason == "Unregistered" {
			status = 410
		}
		err := newAPNsResponseError(status, []byte(`{"reason":"`+reason+`","token":"secret-token"}`))
		if !isPermanentAPNsTokenRejection(err) {
			t.Fatalf("reason %q should be permanent", reason)
		}
		if strings.Contains(err.Error(), "secret-token") {
			t.Fatalf("error exposed device token: %v", err)
		}
	}
	if isPermanentAPNsTokenRejection(newAPNsResponseError(403, []byte(`{"reason":"ExpiredProviderToken"}`))) {
		t.Fatal("provider-token failure must not delete a device registration")
	}
}

func TestTruncateToken(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcdefghijklmnop", "abcdefgh"},
		{"short", "short"},
		{"12345678", "12345678"},
		{"123456789", "12345678"},
	}
	for _, tc := range tests {
		got := truncateToken(tc.input)
		if got != tc.want {
			t.Errorf("truncateToken(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestBoundedHeaderValueRemovesControlsAndPreservesUTF8(t *testing.T) {
	got := boundedHeaderValue(" alert\r\n🧪identifier ", 18)
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("header still contains controls: %q", got)
	}
	if len(got) > 18 || !json.Valid([]byte(`"`+got+`"`)) {
		t.Fatalf("header is not valid bounded UTF-8: %q", got)
	}
}

func TestSanitizedAPNSTransportErrorDoesNotExposeDeviceTokenOrRequestURL(t *testing.T) {
	deviceToken := "private/token-value"
	requestURL := "https://api.push.apple.com/3/device/" + url.PathEscape(deviceToken)
	transportErr := &url.Error{
		Op:  http.MethodPost,
		URL: requestURL,
		Err: errors.New("dial failed while processing " + deviceToken),
	}

	got := sanitizedAPNSTransportError(transportErr, deviceToken)
	if strings.Contains(got, deviceToken) || strings.Contains(got, url.PathEscape(deviceToken)) {
		t.Fatalf("sanitized transport error exposed device token: %q", got)
	}
	if strings.Contains(got, requestURL) {
		t.Fatalf("sanitized transport error exposed APNs request URL: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("sanitized transport error lost useful context: %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
