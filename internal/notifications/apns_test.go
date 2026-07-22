package notifications

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestSignAPNsJWTRejectsNonP256Key(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generate P-384 key: %v", err)
	}
	if _, err := signAPNsJWT(key, "KEYID12345", "TEAMID1234", time.Now()); err == nil {
		t.Fatal("non-P-256 APNs key unexpectedly accepted as ES256")
	}
}

func TestAPNsAuthKeyReadErrorDoesNotExposeHostPath(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "private-host-layout", "AuthKey.p8")
	err := (&APNsAdapter{}).ensureAuthKey(secretPath)
	if err == nil {
		t.Fatal("missing APNs key unexpectedly loaded")
	}
	if strings.Contains(err.Error(), secretPath) || strings.Contains(err.Error(), "private-host-layout") {
		t.Fatalf("APNs auth error exposed host path: %v", err)
	}
}

func TestAPNsAuthKeyRejectsNonRegularAndSymlinkPaths(t *testing.T) {
	directory := t.TempDir()
	tests := []struct {
		name string
		path string
	}{
		{name: "directory", path: directory},
	}

	target := filepath.Join(directory, "target.p8")
	if err := os.WriteFile(target, []byte("not-a-key"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	symlink := filepath.Join(directory, "linked.p8")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	tests = append(tests, struct {
		name string
		path string
	}{name: "symlink", path: symlink})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (&APNsAdapter{}).ensureAuthKey(test.path)
			if err == nil || !strings.Contains(err.Error(), "unavailable or invalid") {
				t.Fatalf("expected bounded non-regular-file error, got %v", err)
			}
			if strings.Contains(err.Error(), test.path) {
				t.Fatalf("APNs auth error exposed host path: %v", err)
			}
		})
	}
}

func TestAPNsAuthKeyRejectsOversizedFileBeforeParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.p8")
	if err := os.WriteFile(path, make([]byte, apnsMaxAuthKeyBytes+1), 0o600); err != nil {
		t.Fatalf("write oversized key: %v", err)
	}
	err := (&APNsAdapter{}).ensureAuthKey(path)
	if err == nil || !strings.Contains(err.Error(), "unavailable or invalid") {
		t.Fatalf("expected oversized-file rejection, got %v", err)
	}
	if strings.Contains(err.Error(), path) {
		t.Fatalf("APNs auth error exposed host path: %v", err)
	}
}

func TestEnsureAPNsJWTCacheIsScopedToKeyAndTeamIdentity(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	adapter := &APNsAdapter{authKey: key}

	first, err := adapter.ensureJWT("KEYID00001", "TEAMID0001")
	if err != nil {
		t.Fatalf("first JWT: %v", err)
	}
	cached, err := adapter.ensureJWT("KEYID00001", "TEAMID0001")
	if err != nil {
		t.Fatalf("cached JWT: %v", err)
	}
	if first != cached {
		t.Fatal("unchanged APNs identity should reuse the cached JWT")
	}

	changedKey, err := adapter.ensureJWT("KEYID00002", "TEAMID0001")
	if err != nil {
		t.Fatalf("changed-key JWT: %v", err)
	}
	if changedKey == cached || adapter.jwtKeyID != "KEYID00002" {
		t.Fatal("changing the APNs key ID must invalidate the cached JWT")
	}

	changedTeam, err := adapter.ensureJWT("KEYID00002", "TEAMID0002")
	if err != nil {
		t.Fatalf("changed-team JWT: %v", err)
	}
	if changedTeam == changedKey || adapter.jwtTeamID != "TEAMID0002" {
		t.Fatal("changing the APNs team ID must invalidate the cached JWT")
	}
}

func TestAPNsAuthorizationJWTKeySelectionIsAtomicAcrossChannels(t *testing.T) {
	t.Parallel()

	type channelKey struct {
		path string
		key  *ecdsa.PrivateKey
		kid  string
		team string
	}
	writeKey := func(name string) channelKey {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("generate %s: %v", name, err)
		}
		encoded, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		path := filepath.Join(t.TempDir(), name+".p8")
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return channelKey{path: path, key: key, kid: name + "KEYID", team: name + "TEAMID"}
	}

	channels := []channelKey{writeKey("FIRST"), writeKey("SECOND")}
	adapter := &APNsAdapter{}
	errCh := make(chan error, 80)
	var wg sync.WaitGroup
	for index := 0; index < 80; index++ {
		channel := channels[index%len(channels)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := adapter.authorizationJWT(channel.path, channel.kid, channel.team)
			if err != nil {
				errCh <- err
				return
			}
			if !verifyAPNsJWT(token, &channel.key.PublicKey) {
				errCh <- fmt.Errorf("JWT was not signed by the selected channel key")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
