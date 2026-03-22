package notifications

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
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
	if aps["category"] != "LT_ALERT_ACTIONS" {
		t.Fatalf("expected default category for alert payload, got %v", aps["category"])
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

func TestSignAPNsJWT(t *testing.T) {
	// Generate a test ECDSA P-256 key.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	now := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	token, err := signAPNsJWT(key, "KEYID12345", "TEAMID1234", now)
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}

	// Verify structure: three dot-separated parts.
	parts := splitJWT(token)
	if parts == nil {
		t.Fatal("JWT should have 3 parts")
	}

	// Decode and verify header.
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "ES256" {
		t.Fatalf("expected alg ES256, got %s", header["alg"])
	}
	if header["kid"] != "KEYID12345" {
		t.Fatalf("expected kid KEYID12345, got %s", header["kid"])
	}

	// Decode and verify claims.
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims["iss"] != "TEAMID1234" {
		t.Fatalf("expected iss TEAMID1234, got %v", claims["iss"])
	}
	iat, ok := claims["iat"].(float64)
	if !ok || int64(iat) != now.Unix() {
		t.Fatalf("expected iat %d, got %v", now.Unix(), claims["iat"])
	}

	// Verify the signature.
	if !verifyAPNsJWT(token, &key.PublicKey) {
		t.Fatal("JWT signature verification failed")
	}
}

func TestSignAPNsJWT_NilKey(t *testing.T) {
	_, err := signAPNsJWT(nil, "KEY", "TEAM", time.Now())
	if err == nil {
		t.Fatal("expected error for nil key")
	}
}

func TestSplitJWT_Invalid(t *testing.T) {
	if splitJWT("no-dots") != nil {
		t.Fatal("expected nil for input without dots")
	}
	if splitJWT("one.dot") != nil {
		t.Fatal("expected nil for input with one dot")
	}
	if splitJWT("too.many.dots.here") != nil {
		t.Fatal("expected nil for input with three dots")
	}
}
