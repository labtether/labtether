package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBuildAPNsPayloadCarriesBoundedNumericDigestCount(t *testing.T) {
	encoded, err := buildAPNsPayload(map[string]any{
		"event":        "alert.digest",
		"title":        "LabTether alert digest",
		"text":         "3 alert updates are ready to review.",
		"digest_count": 3,
	})
	if err != nil {
		t.Fatalf("build digest payload: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode digest payload: %v", err)
	}
	if got, ok := payload["digest_count"].(float64); !ok || got != 3 {
		t.Fatalf("digest_count = %#v, want numeric 3", payload["digest_count"])
	}
	if _, present := payload["alert_id"]; present {
		t.Fatalf("privacy-safe digest unexpectedly contains an alert id: %+v", payload)
	}

	encoded, err = buildAPNsPayload(map[string]any{
		"title": "Digest", "text": "Many updates", "digest_count": 50_000,
	})
	if err != nil {
		t.Fatalf("build bounded digest payload: %v", err)
	}
	payload = nil
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode bounded digest payload: %v", err)
	}
	if got := payload["digest_count"]; got != float64(10_000) {
		t.Fatalf("bounded digest_count = %#v, want 10000", got)
	}
}

func TestAPNsDigestCollapseIDReachesProviderHeader(t *testing.T) {
	originalClient := sharedAPNsHTTPClient
	t.Cleanup(func() { sharedAPNsHTTPClient = originalClient })
	sharedAPNsHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("apns-collapse-id"); got != "digest-stable-1" {
			t.Fatalf("apns-collapse-id = %q, want digest-stable-1", got)
		}
		if got := request.Header.Get("apns-priority"); got != "5" {
			t.Fatalf("apns-priority = %q, want 5", got)
		}
		if got := request.Header.Get("apns-expiration"); got != "1780000000" {
			t.Fatalf("apns-expiration = %q, want 1780000000", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})}

	adapter := &APNsAdapter{}
	if err := adapter.sendToDevice(
		context.Background(),
		apnsSandboxEndpoint,
		"provider-token",
		"com.labtether.mobile",
		"provider-jwt",
		[]byte(`{"aps":{}}`),
		map[string]any{
			"event": "alert.digest", "collapse_id": "digest-stable-1",
			"apns_priority": 5, "apns_expiration_unix": int64(1_780_000_000),
		},
	); err != nil {
		t.Fatalf("send digest: %v", err)
	}
}
