package notifications

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	apnsProductionEndpoint      = "https://api.push.apple.com"
	apnsSandboxEndpoint         = "https://api.sandbox.push.apple.com"
	apnsInvalidTokenHandlerKey  = "_labtether_invalid_apns_token_handler" // #nosec G101 -- Private in-process map key name, not an APNs token value.
	apnsMaxConcurrentDeliveries = 8
	apnsMaxAuthKeyBytes         = 16 << 10
)

// Reuse one concurrency-safe client so APNs fanout can benefit from pooled
// HTTP/2 connections instead of establishing a new connection per device.
var sharedAPNsHTTPClient = &http.Client{Timeout: 15 * time.Second}

// APNsInvalidDeviceTokenHandler removes a registration after APNs confirms
// that the token can no longer be used for the selected topic/environment.
// The token is passed only to this in-process callback and is never included
// in the adapter's persisted/loggable error text.
type APNsInvalidDeviceTokenHandler func(deviceToken string) error

// SetAPNsInvalidDeviceTokenHandler attaches a trusted runtime-only callback to
// an APNs config clone. Notification channel config persisted by operators must
// never contain this value.
func SetAPNsInvalidDeviceTokenHandler(config map[string]any, handler APNsInvalidDeviceTokenHandler) {
	if config == nil || handler == nil {
		return
	}
	config[apnsInvalidTokenHandlerKey] = handler
}

// APNsAdapter sends push notifications via Apple Push Notification Service.
//
// Channel config keys:
//   - auth_key_path: path to .p8 auth key file (ECDSA P-256 private key)
//   - key_id:        Apple Key ID (10-character identifier)
//   - team_id:       Apple Team ID (10-character identifier)
//   - bundle_id:     app bundle identifier (e.g. "com.labtether.mobile")
//   - production:    bool — true for production APNs endpoint, false for sandbox
//   - device_tokens: []string — populated by dispatch code from push_devices table
//   - transport:     "direct" (operator key) or "relay"; absent means relay
//     when auth_key_path, key_id and team_id are all blank
//
// Payload keys consumed:
//   - title: alert title string
//   - text:  alert body string
//   - alert_id: optional alert identifier for collapse grouping
//   - severity, status, summary, event, rule_id: optional routing metadata forwarded to iOS
type APNsAdapter struct {
	// Relay delivers channels without an operator key through the LabTether
	// push relay. Nil disables relay mode (LABTETHER_PUSH_RELAY_URL=off).
	Relay *APNsRelayClient

	mu        sync.Mutex
	authKey   *ecdsa.PrivateKey
	keyPath   string // tracks which file was loaded
	jwtToken  string
	jwtExp    time.Time
	jwtKeyID  string
	jwtTeamID string
}

// apnsDeliveryBatchError retains the positional outcome of a multi-device
// APNs request without retaining or exposing bearer-style device tokens. The
// hub uses these indices to retry only the failed registrations from a group.
type apnsDeliveryBatchError struct {
	total    int
	failures []apnsIndexedDeliveryFailure
}

type apnsIndexedDeliveryFailure struct {
	index int
	err   error
}

func (e *apnsDeliveryBatchError) Error() string {
	if e == nil {
		return "apns: delivery failed"
	}
	details := make([]string, 0, len(e.failures))
	for _, failure := range e.failures {
		details = append(details, fmt.Sprintf("delivery %d: %v", failure.index+1, failure.err))
	}
	return fmt.Sprintf("apns: %d/%d deliveries failed: %s", len(e.failures), e.total, strings.Join(details, "; "))
}

func (e *apnsDeliveryBatchError) FailedDeliveryIndices() []int {
	if e == nil || len(e.failures) == 0 {
		return nil
	}
	indices := make([]int, 0, len(e.failures))
	for _, failure := range e.failures {
		indices = append(indices, failure.index)
	}
	return indices
}

// APNsFailedDeliveryIndices extracts positional failures from an APNs adapter
// error. The returned indices correspond to the device_tokens slice supplied
// to Adapter.Send. Callers must validate every index before using it.
func APNsFailedDeliveryIndices(err error) ([]int, bool) {
	if err == nil {
		return nil, false
	}
	var indexed interface{ FailedDeliveryIndices() []int }
	if !errors.As(err, &indexed) {
		return nil, false
	}
	indices := indexed.FailedDeliveryIndices()
	return append([]int(nil), indices...), true
}

func (a *APNsAdapter) Type() string { return ChannelTypeAPNs }

func (a *APNsAdapter) Send(ctx context.Context, config map[string]any, payload map[string]any) error {
	// A channel without an operator key delivers through the LabTether push
	// relay; its device tokens are relay registration ids, not APNs tokens.
	if APNsUsesRelay(config) {
		return a.sendViaRelay(ctx, config, payload)
	}
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

	// Select the key and sign/read its JWT under one lock. Keeping these two
	// operations atomic prevents concurrent channels with different .p8 files
	// from signing one channel's JWT with another channel's key.
	jwt, err := a.authorizationJWT(authKeyPath, keyID, teamID)
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
	return deliverAPNsBatch(ctx, config, tokens, func(ctx context.Context, token string) error {
		return a.sendToDevice(ctx, endpoint, token, bundleID, jwt, apnsPayload, payload)
	})
}

// deliverAPNsBatch sends to each token through a fixed worker pool. APNs
// supports HTTP/2 multiplexing, while a goroutine-per-token fanout lets an
// oversized device registry amplify one alert into unbounded runnable
// goroutines even when a semaphore bounds the requests actually in flight.
// Direct and relay delivery share the permanent-rejection cleanup handler and
// the positional batch error used for targeted retries.
func deliverAPNsBatch(
	ctx context.Context,
	config map[string]any,
	tokens []string,
	deliver func(context.Context, string) error,
) error {
	deliveryErrors := make([]error, len(tokens))
	workerCount := apnsMaxConcurrentDeliveries
	if len(tokens) < workerCount {
		workerCount = len(tokens)
	}
	jobs := make(chan int)
	var deliveries sync.WaitGroup
	deliveries.Add(workerCount)
	for range workerCount {
		go func() {
			defer deliveries.Done()
			for index := range jobs {
				token := tokens[index]
				sendErr := deliver(ctx, token)
				if sendErr == nil {
					continue
				}
				cleanupSuffix := ""
				if isPermanentAPNsTokenRejection(sendErr) {
					if handler, ok := config[apnsInvalidTokenHandlerKey].(APNsInvalidDeviceTokenHandler); ok {
						if cleanupErr := handler(token); cleanupErr != nil {
							cleanupSuffix = " (permanent rejection cleanup failed)"
						}
					}
				}
				// Device tokens are bearer-style delivery credentials. Never include
				// even a prefix in an error that can be persisted or logged.
				deliveryErrors[index] = fmt.Errorf("%v%s", sendErr, cleanupSuffix)
			}
		}()
	}

	nextIndex := 0
dispatch:
	for ; nextIndex < len(tokens); nextIndex++ {
		select {
		case jobs <- nextIndex:
		case <-ctx.Done():
			for index := nextIndex; index < len(tokens); index++ {
				deliveryErrors[index] = ctx.Err()
			}
			break dispatch
		}
	}
	close(jobs)
	deliveries.Wait()

	failures := make([]apnsIndexedDeliveryFailure, 0)
	for index, deliveryErr := range deliveryErrors {
		if deliveryErr != nil {
			failures = append(failures, apnsIndexedDeliveryFailure{index: index, err: deliveryErr})
		}
	}
	if len(failures) > 0 {
		return &apnsDeliveryBatchError{total: len(tokens), failures: failures}
	}
	return nil
}

// sendToDevice performs a single HTTP/2 POST to the APNs endpoint for one device token.
func (a *APNsAdapter) sendToDevice(ctx context.Context, endpoint, token, bundleID, jwt string, body []byte, payload map[string]any) error {
	url := fmt.Sprintf("%s/3/device/%s", endpoint, url.PathEscape(token))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", bundleID)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", apnsPriorityHeader(apnsAlertPriority(payload)))
	if expiration := payloadUnixTime(payload, "apns_expiration_unix"); expiration > 0 {
		req.Header.Set("apns-expiration", strconv.FormatInt(expiration, 10))
	}

	// Callers may supply a privacy-safe collapse identifier when no alert or
	// incident identifier belongs in the payload (for example a digest).
	if collapseID := apnsAlertCollapseID(payload); collapseID != "" {
		req.Header.Set("apns-collapse-id", boundedHeaderValue(collapseID, 64))
	}

	resp, err := sharedAPNsHTTPClient.Do(req) // #nosec G704 -- APNS endpoint is the fixed Apple push host selected by environment mode.
	if err != nil {
		return fmt.Errorf("request failed: %s", sanitizedAPNSTransportError(err, token))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	// Read APNs error response body for diagnostics and permanent-token cleanup.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return newAPNsResponseError(resp.StatusCode, respBody)
}

// sanitizedAPNSTransportError preserves useful network diagnostics while
// removing the APNs request URL, whose path contains the bearer-style device
// token. Exact and URL-escaped token forms are redacted defensively as well.
func sanitizedAPNSTransportError(err error, deviceToken string) string {
	if err == nil {
		return "unknown transport error"
	}
	var urlError *url.Error
	if errors.As(err, &urlError) && urlError.Err != nil {
		err = urlError.Err
	}
	message := err.Error()
	for _, secret := range []string{deviceToken, url.PathEscape(deviceToken)} {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	if message = boundedUTF8(message, 256); message == "" {
		return "unknown transport error"
	}
	return message
}

type apnsResponseError struct {
	statusCode int
	reason     string
}

func (e *apnsResponseError) Error() string {
	return fmt.Sprintf("status %d: %s", e.statusCode, e.reason)
}

func newAPNsResponseError(statusCode int, body []byte) error {
	var response struct {
		Reason string `json:"reason"`
	}
	reason := "unknown APNs rejection"
	if json.Unmarshal(body, &response) == nil {
		if candidate := boundedUTF8(response.Reason, 128); candidate != "" {
			reason = candidate
		}
	}
	return &apnsResponseError{statusCode: statusCode, reason: reason}
}

func isPermanentAPNsTokenRejection(err error) bool {
	var responseError *apnsResponseError
	if !errors.As(err, &responseError) {
		return false
	}
	switch responseError.reason {
	case "BadDeviceToken", "DeviceTokenNotForTopic":
		return responseError.statusCode == http.StatusBadRequest
	case "Unregistered":
		return responseError.statusCode == http.StatusGone
	default:
		return false
	}
}

// ensureAuthKey loads the .p8 ECDSA private key from disk, caching it for reuse.
// If the path changes, the key is reloaded.
func (a *APNsAdapter) ensureAuthKey(path string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.authKey != nil && a.keyPath == path {
		return nil
	}
	return a.loadAuthKeyLocked(path)
}

// authorizationJWT atomically selects the configured key and obtains a JWT
// scoped to that key/team identity. APNsAdapter is shared across notification
// channels, so the key load and JWT lookup must not be separate lock epochs.
func (a *APNsAdapter) authorizationJWT(path, keyID, teamID string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.authKey == nil || a.keyPath != path {
		if err := a.loadAuthKeyLocked(path); err != nil {
			return "", err
		}
	}
	return a.ensureJWTLocked(keyID, teamID)
}

func (a *APNsAdapter) loadAuthKeyLocked(path string) error {
	// Reject devices, FIFOs, symlinks and oversized files before opening the
	// operator-configured path. In particular, /dev/zero or a named pipe must not
	// turn a channel test into an unbounded read or a blocked hub process.
	pathInfo, err := os.Lstat(path) // #nosec G304 -- API validation requires a bounded absolute .p8 path; runtime still validates the file itself.
	if err != nil || !pathInfo.Mode().IsRegular() || pathInfo.Size() <= 0 || pathInfo.Size() > apnsMaxAuthKeyBytes {
		return errors.New("apns: auth key file is unavailable or invalid")
	}
	file, err := os.Open(path) // #nosec G304 -- The path and file type are checked immediately before and after open.
	if err != nil {
		// Channel delivery errors can be persisted and surfaced to operators.
		// Do not disclose a host filesystem path through that error boundary.
		return errors.New("apns: auth key file is unavailable")
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Size() <= 0 || openedInfo.Size() > apnsMaxAuthKeyBytes {
		return errors.New("apns: auth key file is unavailable or invalid")
	}
	keyData, err := io.ReadAll(io.LimitReader(file, apnsMaxAuthKeyBytes+1))
	if err != nil || len(keyData) == 0 || len(keyData) > apnsMaxAuthKeyBytes {
		return errors.New("apns: auth key file is unavailable or invalid")
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return fmt.Errorf("apns: auth key file contains no PEM data")
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
	a.jwtKeyID = ""
	a.jwtTeamID = ""
	return nil
}

// ensureJWT returns a valid JWT, generating a new one if the current token is
// expired or missing. Apple recommends refreshing APNs JWTs no more than once
// every 20 minutes and they expire after 60 minutes.
func (a *APNsAdapter) ensureJWT(keyID, teamID string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ensureJWTLocked(keyID, teamID)
}

func (a *APNsAdapter) ensureJWTLocked(keyID, teamID string) (string, error) {
	if a.jwtToken != "" && keyID == a.jwtKeyID && teamID == a.jwtTeamID && time.Now().Before(a.jwtExp) {
		return a.jwtToken, nil
	}

	now := time.Now()
	token, err := signAPNsJWT(a.authKey, keyID, teamID, now)
	if err != nil {
		return "", err
	}

	a.jwtToken = token
	a.jwtExp = now.Add(50 * time.Minute) // refresh well before 60-minute expiry
	a.jwtKeyID = keyID
	a.jwtTeamID = teamID
	return token, nil
}
