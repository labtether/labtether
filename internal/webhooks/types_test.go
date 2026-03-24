package webhooks

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ── Webhook JSON serialization ────────────────────────────────────────────────

// TestWebhook_SecretOmittedFromJSON verifies that the Secret field is never
// included in JSON output (it carries json:"-").
func TestWebhook_SecretOmittedFromJSON(t *testing.T) {
	wh := Webhook{
		ID:      "wh_001",
		Name:    "My Hook",
		URL:     "https://example.com/hook",
		Secret:  "super-secret-value",
		Events:  []string{"alert.fired"},
		Enabled: true,
	}
	b, err := json.Marshal(wh)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(b), "super-secret-value") {
		t.Errorf("JSON output contains Secret value: %s", b)
	}
	if strings.Contains(string(b), `"secret"`) {
		t.Errorf("JSON output contains secret key: %s", b)
	}
}

// TestWebhook_RoundTrip verifies that exported fields survive a marshal →
// unmarshal round-trip without data loss.
func TestWebhook_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	wh := Webhook{
		ID:        "wh_round",
		Name:      "Round-trip Hook",
		URL:       "https://example.com/rt",
		Secret:    "ignored",
		Events:    []string{"asset.online", "asset.offline"},
		Enabled:   true,
		CreatedBy: "user_123",
		CreatedAt: now,
	}

	b, err := json.Marshal(wh)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Webhook
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != wh.ID {
		t.Errorf("ID = %q, want %q", got.ID, wh.ID)
	}
	if got.Name != wh.Name {
		t.Errorf("Name = %q, want %q", got.Name, wh.Name)
	}
	if got.URL != wh.URL {
		t.Errorf("URL = %q, want %q", got.URL, wh.URL)
	}
	if got.Enabled != wh.Enabled {
		t.Errorf("Enabled = %v, want %v", got.Enabled, wh.Enabled)
	}
	if got.CreatedBy != wh.CreatedBy {
		t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, wh.CreatedBy)
	}
	if len(got.Events) != len(wh.Events) {
		t.Errorf("Events len = %d, want %d", len(got.Events), len(wh.Events))
	}
	// Secret must NOT round-trip (json:"-")
	if got.Secret != "" {
		t.Errorf("Secret should be empty after unmarshal, got %q", got.Secret)
	}
}

// TestWebhook_LastTriggeredAtOmittedWhenNil verifies that the
// last_triggered_at field is absent from JSON when it is nil.
func TestWebhook_LastTriggeredAtOmittedWhenNil(t *testing.T) {
	wh := Webhook{
		ID:              "wh_niltime",
		Name:            "Hook",
		URL:             "https://example.com/h",
		LastTriggeredAt: nil,
	}
	b, err := json.Marshal(wh)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(b), "last_triggered_at") {
		t.Errorf("JSON should not contain last_triggered_at when nil: %s", b)
	}
}

// TestWebhook_LastTriggeredAtPresentWhenSet verifies that last_triggered_at
// appears in JSON when a non-nil pointer is set.
func TestWebhook_LastTriggeredAtPresentWhenSet(t *testing.T) {
	ts := time.Now().UTC()
	wh := Webhook{
		ID:              "wh_ts",
		LastTriggeredAt: &ts,
	}
	b, err := json.Marshal(wh)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(b), "last_triggered_at") {
		t.Errorf("JSON should contain last_triggered_at when set: %s", b)
	}
}

// TestWebhook_DisabledDefaultsToFalse verifies that a zero-value Webhook has
// Enabled=false (the Go zero value for bool).
func TestWebhook_DisabledDefaultsToFalse(t *testing.T) {
	var wh Webhook
	if wh.Enabled {
		t.Error("zero-value Webhook.Enabled should be false")
	}
}

// ── CreateRequest JSON serialization ─────────────────────────────────────────

// TestCreateRequest_SecretOmittedWhenEmpty verifies that an empty secret is
// omitted from JSON output (json:"secret,omitempty").
func TestCreateRequest_SecretOmittedWhenEmpty(t *testing.T) {
	req := CreateRequest{
		Name:   "hook",
		URL:    "https://example.com/h",
		Events: []string{"asset.online"},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(b), `"secret"`) {
		t.Errorf("JSON should not contain secret key when empty: %s", b)
	}
}

// TestCreateRequest_SecretPresentWhenSet verifies that a non-empty secret IS
// included in JSON (it is provided at the boundary where the client sends it).
func TestCreateRequest_SecretPresentWhenSet(t *testing.T) {
	req := CreateRequest{
		Name:   "hook",
		URL:    "https://example.com/h",
		Secret: "mysecret",
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(b), "mysecret") {
		t.Errorf("JSON should contain secret when set: %s", b)
	}
}

// TestCreateRequest_RoundTrip verifies that all fields survive a
// marshal → unmarshal round-trip.
func TestCreateRequest_RoundTrip(t *testing.T) {
	req := CreateRequest{
		Name:   "Test Hook",
		URL:    "https://example.com/hook",
		Secret: "tok3n",
		Events: []string{"alert.fired", "asset.online"},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got CreateRequest
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Name != req.Name {
		t.Errorf("Name = %q, want %q", got.Name, req.Name)
	}
	if got.URL != req.URL {
		t.Errorf("URL = %q, want %q", got.URL, req.URL)
	}
	if got.Secret != req.Secret {
		t.Errorf("Secret = %q, want %q", got.Secret, req.Secret)
	}
	if len(got.Events) != len(req.Events) {
		t.Errorf("Events len = %d, want %d", len(got.Events), len(req.Events))
	}
}

// TestCreateRequest_UnmarshalFromJSON tests that the struct can be populated
// from a raw JSON string as a client would send it.
func TestCreateRequest_UnmarshalFromJSON(t *testing.T) {
	raw := `{"name":"My Hook","url":"https://hooks.example.com/recv","events":["alert.fired","asset.offline"],"secret":"s3cr3t"}`
	var req CreateRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Name != "My Hook" {
		t.Errorf("Name = %q, want %q", req.Name, "My Hook")
	}
	if req.URL != "https://hooks.example.com/recv" {
		t.Errorf("URL = %q", req.URL)
	}
	if req.Secret != "s3cr3t" {
		t.Errorf("Secret = %q, want %q", req.Secret, "s3cr3t")
	}
	if len(req.Events) != 2 {
		t.Errorf("Events len = %d, want 2", len(req.Events))
	}
}
