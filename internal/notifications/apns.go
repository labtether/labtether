package notifications

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	apnsProductionEndpoint = "https://api.push.apple.com"
	apnsSandboxEndpoint    = "https://api.sandbox.push.apple.com"
)

// APNsAdapter sends push notifications via Apple Push Notification Service.
//
// Channel config keys:
//   - auth_key_path: path to .p8 auth key file (ECDSA P-256 private key)
//   - key_id:        Apple Key ID (10-character identifier)
//   - team_id:       Apple Team ID (10-character identifier)
//   - bundle_id:     app bundle identifier (e.g. "com.labtether.mobile")
//   - production:    bool — true for production APNs endpoint, false for sandbox
//   - device_tokens: []string — populated by dispatch code from push_devices table
//
// Payload keys consumed:
//   - title: alert title string
//   - text:  alert body string
//   - alert_id: optional alert identifier for collapse grouping
type APNsAdapter struct {
	mu       sync.Mutex
	authKey  *ecdsa.PrivateKey
	keyPath  string // tracks which file was loaded
	jwtToken string
	jwtExp   time.Time
}

func (a *APNsAdapter) Type() string { return ChannelTypeAPNs }

func (a *APNsAdapter) Send(ctx context.Context, config map[string]any, payload map[string]any) error {
	// Extract and validate required config fields.
	authKeyPath, _ := config["auth_key_path"].(string)
	keyID, _ := config["key_id"].(string)
	teamID, _ := config["team_id"].(string)
	bundleID, _ := config["bundle_id"].(string)

	if authKeyPath == "" || keyID == "" || teamID == "" || bundleID == "" {
		return fmt.Errorf("apns: incomplete configuration — auth_key_path, key_id, team_id, and bundle_id are all required")
	}

	// Determine device tokens to deliver to.
	tokens := extractDeviceTokens(config)
	if len(tokens) == 0 {
		// No registered devices — nothing to send, not an error.
		return nil
	}

	// Load (or reload) the auth key.
	if err := a.ensureAuthKey(authKeyPath); err != nil {
		return err
	}

	// Refresh JWT if expired or not yet generated.
	jwt, err := a.ensureJWT(keyID, teamID)
	if err != nil {
		return err
	}

	// Build the APNs JSON payload.
	apnsPayload, err := buildAPNsPayload(payload)
	if err != nil {
		return fmt.Errorf("apns: build payload: %w", err)
	}

	production, _ := config["production"].(bool)
	endpoint := apnsSandboxEndpoint
	if production {
		endpoint = apnsProductionEndpoint
	}

	// Send to each device token. Collect errors but attempt all.
	var errs []string
	for _, token := range tokens {
		if sendErr := a.sendToDevice(ctx, endpoint, token, bundleID, jwt, apnsPayload, payload); sendErr != nil {
			errs = append(errs, fmt.Sprintf("token %s…: %v", truncateToken(token), sendErr))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("apns: %d/%d deliveries failed: %s", len(errs), len(tokens), strings.Join(errs, "; "))
	}
	return nil
}

// sendToDevice performs a single HTTP/2 POST to the APNs endpoint for one device token.
func (a *APNsAdapter) sendToDevice(ctx context.Context, endpoint, token, bundleID, jwt string, body []byte, payload map[string]any) error {
	url := fmt.Sprintf("%s/3/device/%s", endpoint, token)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", bundleID)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")

	// Use alert_id as collapse identifier if present.
	if alertID, ok := payload["alert_id"].(string); ok && alertID != "" {
		req.Header.Set("apns-collapse-id", alertID)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req) // #nosec G704 -- APNS endpoint is the fixed Apple push host selected by environment mode.
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	// Read APNs error response body for diagnostics.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
}

// ensureAuthKey loads the .p8 ECDSA private key from disk, caching it for reuse.
// If the path changes, the key is reloaded.
func (a *APNsAdapter) ensureAuthKey(path string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.authKey != nil && a.keyPath == path {
		return nil
	}

	keyData, err := os.ReadFile(path) // #nosec G304 -- Path comes from reviewed APNS auth-key config, not untrusted user input.
	if err != nil {
		return fmt.Errorf("apns: read auth key %s: %w", path, err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return fmt.Errorf("apns: auth key file %s contains no PEM data", path)
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("apns: parse auth key: %w", err)
	}

	ecKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return fmt.Errorf("apns: auth key is not an ECDSA key (got %T)", parsed)
	}

	a.authKey = ecKey
	a.keyPath = path
	a.jwtToken = "" // force JWT regeneration with new key
	a.jwtExp = time.Time{}
	return nil
}

// ensureJWT returns a valid JWT, generating a new one if the current token is
// expired or missing. Apple recommends refreshing APNs JWTs no more than once
// every 20 minutes and they expire after 60 minutes.
func (a *APNsAdapter) ensureJWT(keyID, teamID string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.jwtToken != "" && time.Now().Before(a.jwtExp) {
		return a.jwtToken, nil
	}

	now := time.Now()
	token, err := signAPNsJWT(a.authKey, keyID, teamID, now)
	if err != nil {
		return "", err
	}

	a.jwtToken = token
	a.jwtExp = now.Add(50 * time.Minute) // refresh well before 60-minute expiry
	return token, nil
}

// buildAPNsPayload constructs the APNs JSON payload from the notification payload.
func buildAPNsPayload(payload map[string]any) ([]byte, error) {
	title := payloadString(payload, "title")
	text := payloadString(payload, "text")
	if title == "" {
		title = "LabTether Alert"
	}

	aps := map[string]any{
		"alert": map[string]any{
			"title": title,
			"body":  text,
		},
		"sound": "default",
	}

	alertID := firstNonBlank(
		payloadString(payload, "alert_id"),
		payloadString(payload, "alert_instance_id"),
	)
	apns := map[string]any{"aps": aps}
	if alertID != "" {
		apns["alert_id"] = alertID
	}

	category := payloadString(payload, "apns_category")
	if category == "" && alertID != "" {
		category = "LT_ALERT_ACTIONS"
	}
	if category != "" {
		aps["category"] = category
	}

	for _, key := range []string{"incident_id", "deep_link", "runbook_id"} {
		if value := payloadString(payload, key); value != "" {
			apns[key] = value
		}
	}

	return json.Marshal(apns)
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// extractDeviceTokens reads device tokens from the config map.
// Supports both []string and []any (as JSON-decoded arrays typically arrive as []any).
func extractDeviceTokens(config map[string]any) []string {
	if tokens, ok := config["device_tokens"].([]string); ok {
		return tokens
	}
	raw, ok := config["device_tokens"].([]any)
	if !ok {
		return nil
	}
	tokens := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			tokens = append(tokens, s)
		}
	}
	return tokens
}

// truncateToken returns the first 8 characters of a device token for safe logging.
func truncateToken(token string) string {
	if len(token) > 8 {
		return token[:8]
	}
	return token
}
