package notifications

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBuildLiveActivityPayloadMatchesActivityKitContract(t *testing.T) {
	started := time.Date(2026, 7, 14, 2, 0, 0, 0, time.UTC)
	updated := started.Add(5 * time.Minute)
	stale := updated.Add(20 * time.Minute)
	body, err := buildLiveActivityPayload(LiveActivityPush{
		Event:     "update",
		Timestamp: updated,
		StaleAt:   &stale,
		State: LiveActivityContentState{
			Title:           "Database outage",
			Summary:         "Primary unavailable",
			Status:          "investigating",
			Severity:        "critical",
			Assignee:        "OnCall",
			StartedAt:       started,
			UpdatedAt:       updated,
			ShowFullDetails: true,
			CanMutate:       true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	aps := payload["aps"].(map[string]any)
	if aps["event"] != "update" || int64(aps["timestamp"].(float64)) != updated.Unix() {
		t.Fatalf("unexpected ActivityKit envelope: %#v", aps)
	}
	state := aps["content-state"].(map[string]any)
	if state["title"] != "Database outage" || state["canMutate"] != true {
		t.Fatalf("unexpected content state: %#v", state)
	}
	if got := state["startedAt"].(float64); got != foundationDateValue(started) {
		t.Fatalf("startedAt=%v want %v", got, foundationDateValue(started))
	}
	if _, found := aps["alert"]; found {
		t.Fatal("liveactivity payload must not contain a notification alert")
	}
}

func TestBuildLiveActivityEndPayloadIncludesDismissalDate(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	body, err := buildLiveActivityPayload(LiveActivityPush{
		Event:     "end",
		Timestamp: now,
		DismissAt: &now,
		State: LiveActivityContentState{
			Title: "Incident", Status: "resolved", Severity: "high",
			StartedAt: now.Add(-time.Hour), UpdatedAt: now,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"event":"end"`) || !strings.Contains(string(body), `"dismissal-date"`) {
		t.Fatalf("unexpected end payload: %s", body)
	}
}

func TestSendLiveActivityUsesDedicatedHeadersAndRedactsTransportToken(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &APNsAdapter{authKey: key, keyPath: "test-key.p8"}
	token := strings.Repeat("ab", 32)
	original := sharedAPNsHTTPClient
	t.Cleanup(func() { sharedAPNsHTTPClient = original })
	sharedAPNsHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("apns-push-type") != "liveactivity" {
			t.Fatalf("push type=%q", req.Header.Get("apns-push-type"))
		}
		if req.Header.Get("apns-topic") != "com.labtether.mobile.push-type.liveactivity" {
			t.Fatalf("topic=%q", req.Header.Get("apns-topic"))
		}
		if req.Header.Get("apns-collapse-id") != "activity-1" {
			t.Fatalf("collapse id=%q", req.Header.Get("apns-collapse-id"))
		}
		if req.Header.Get("apns-priority") != "5" {
			t.Fatalf("priority=%q", req.Header.Get("apns-priority"))
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	now := time.Now().UTC()
	err = adapter.SendLiveActivity(context.Background(), map[string]any{
		"auth_key_path": "test-key.p8",
		"key_id":        "KEYID12345",
		"team_id":       "TEAMID1234",
		"bundle_id":     "com.labtether.mobile",
	}, LiveActivityPush{
		DeviceToken: token,
		BundleID:    "com.labtether.mobile",
		ActivityID:  "activity-1",
		Event:       "update",
		Priority:    5,
		Timestamp:   now,
		State: LiveActivityContentState{
			Title: "Incident", Status: "open", Severity: "critical", StartedAt: now, UpdatedAt: now,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidLiveActivityPushTokenAcceptsBoundedOpaqueHex(t *testing.T) {
	for _, valid := range []string{"a0", strings.Repeat("A0", 32), strings.Repeat("ab", 100)} {
		if !validLiveActivityPushToken(valid) {
			t.Fatalf("expected bounded hexadecimal token to be accepted: %d bytes", len(valid)/2)
		}
	}
	for _, invalid := range []string{"", "abc", strings.Repeat("zz", 32), strings.Repeat("aa", 101)} {
		if validLiveActivityPushToken(invalid) {
			t.Fatalf("accepted invalid token length/content %d", len(invalid))
		}
	}
}
