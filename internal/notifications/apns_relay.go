package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

// The LabTether push relay (services/push-relay) holds LabTether's APNs key so
// self-hosted Hubs can reach the official iOS app. The app registers its APNs
// token with the relay and gives the Hub only an opaque registration id. The
// Hub treats that id exactly like a device token: a bearer credential that is
// never logged, never placed in an error and never changed in case.
const (
	APNsTransportDirect = "direct"
	APNsTransportRelay  = "relay"
	DefaultPushRelayURL = "https://push.labtether.com"
	OfficialAppBundleID = "com.labtether.mobile"

	apnsRelayTimeout          = 15 * time.Second
	apnsRelayMaxResponseBytes = 1 << 10
)

var relayRegistrationIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// ValidRelayRegistrationID reports whether id has the relay's wire format:
// 256 random bits as unpadded base64url. The check is case-sensitive.
func ValidRelayRegistrationID(id string) bool {
	return relayRegistrationIDPattern.MatchString(id)
}

// NormalizeAPNsTransport maps a stored transport value to direct or relay.
// Anything other than relay is direct, which keeps legacy rows on the
// operator-key path they were registered for.
func NormalizeAPNsTransport(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), APNsTransportRelay) {
		return APNsTransportRelay
	}
	return APNsTransportDirect
}

// APNsUsesRelay reports whether an APNs channel config delivers through the
// relay: an explicit transport wins, otherwise a channel with no operator key
// (auth_key_path, key_id and team_id all blank) uses the relay.
func APNsUsesRelay(config map[string]any) bool {
	transport, _ := config["transport"].(string)
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case APNsTransportRelay:
		return true
	case APNsTransportDirect:
		return false
	}
	for _, key := range []string{"auth_key_path", "key_id", "team_id"} {
		if value, _ := config[key].(string); strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

// ParsePushRelayURL validates LABTETHER_PUSH_RELAY_URL. Empty, "off",
// "false", "disabled" and "none" disable relay mode and return "".
func ParsePushRelayURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	switch strings.ToLower(trimmed) {
	case "", "off", "false", "disabled", "none":
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" || parsed.Hostname() == "" {
		return "", errors.New("push relay URL must be an absolute URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && (scheme != "http" || !securityruntime.InsecureTransportAllowed()) {
		return "", errors.New("push relay URL must use https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", errors.New("push relay URL must not contain credentials, a query or a fragment")
	}
	parsed.Scheme = scheme
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed.String(), nil
}

// APNsRelayClient posts pushes to {baseURL}/v1/push through the outbound
// security runtime. The HTTP client is restricted to the relay origin, never
// follows redirects and is built lazily, so a Hub that starts offline still
// starts and simply retries delivery later.
type APNsRelayClient struct {
	pushURL string

	mu     sync.Mutex
	client *http.Client
}

// NewAPNsRelayClient returns nil for an empty base URL (relay disabled).
// baseURL must already be validated with ParsePushRelayURL.
func NewAPNsRelayClient(baseURL string) *APNsRelayClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	return &APNsRelayClient{pushURL: baseURL + "/v1/push"}
}

// RelayEnabled reports whether this adapter can deliver relay channels.
func (a *APNsAdapter) RelayEnabled() bool {
	return a != nil && a.Relay != nil
}

// apnsRelayPush is the POST /v1/push body. Payload is the complete APNs JSON
// object the Hub already built and bounded for direct delivery.
type apnsRelayPush struct {
	RegistrationID string          `json:"registration_id"`
	PushType       string          `json:"push_type"`
	Priority       int64           `json:"priority"`
	Expiration     int64           `json:"expiration,omitempty"`
	CollapseID     string          `json:"collapse_id,omitempty"`
	Payload        json.RawMessage `json:"payload"`
}

// The relay keeps the address policy of the process: these flags only stop
// this client from being stricter than LABTETHER_OUTBOUND_* already is.
var apnsRelayAddressPolicy = securityruntime.OutboundAddressPolicy{AllowPrivate: true, AllowLoopback: true}

func (c *APNsRelayClient) httpClient() (*http.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	base := &http.Client{
		Timeout: apnsRelayTimeout,
		// A redirect would resend the body, which carries a bearer id.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	client, err := securityruntime.NewOriginRestrictedOutboundHTTPClient(base, apnsRelayAddressPolicy, c.pushURL)
	if err != nil {
		return nil, err
	}
	c.client = client
	return client, nil
}

func (c *APNsRelayClient) push(ctx context.Context, request apnsRelayPush) error {
	body, err := json.Marshal(request)
	if err != nil {
		return errors.New("push relay: encode request failed")
	}
	client, err := c.httpClient()
	if err != nil {
		return fmt.Errorf("push relay unavailable: %s", boundedUTF8(err.Error(), 256))
	}
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodPost, c.pushURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("push relay unavailable: %s", boundedUTF8(err.Error(), 256))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req) // #nosec G704 -- The relay origin is validated at startup and enforced on every request and dial by the origin-restricted client.
	if err != nil {
		return fmt.Errorf("push relay unavailable: %s", sanitizedAPNSTransportError(err, request.RegistrationID))
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, apnsRelayMaxResponseBytes))
	return relayResponseError(resp.StatusCode, responseBody)
}

// relayResponseError maps a relay answer onto the errors direct delivery
// uses. Only the relay's explicit "registration is gone" answers become a
// permanent rejection, so a misconfigured URL answering a bare 404 can never
// trigger mass deletion of registrations.
func relayResponseError(status int, body []byte) error {
	var answer struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(body, &answer)
	code := relayErrorCode(answer.Error)
	switch {
	case status == http.StatusOK:
		return nil
	case status == http.StatusGone && code == "unregistered",
		status == http.StatusNotFound && code == "unknown_registration":
		return &apnsResponseError{statusCode: http.StatusGone, reason: "Unregistered"}
	case status == http.StatusBadGateway && code == "apns_error":
		reason := boundedUTF8(answer.Reason, 128)
		if reason == "" {
			reason = "unknown APNs rejection"
		}
		return &apnsResponseError{statusCode: http.StatusBadGateway, reason: reason}
	case status == http.StatusTooManyRequests:
		return errors.New("push relay rate limited")
	case status >= http.StatusInternalServerError:
		return fmt.Errorf("push relay unavailable: %s", strings.TrimSpace(fmt.Sprintf("status %d %s", status, code)))
	default:
		return fmt.Errorf("push relay rejected request: %s", strings.TrimSpace(fmt.Sprintf("status %d %s", status, code)))
	}
}

// relayErrorCode keeps only a short snake_case code from the relay answer.
func relayErrorCode(value string) string {
	if len(value) == 0 || len(value) > 64 {
		return ""
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && r != '_' {
			return ""
		}
	}
	return value
}

// relayCollapseID keeps only printable ASCII, which is what the relay (and
// Apple's apns-collapse-id header) accepts, capped at 64 bytes.
func relayCollapseID(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if len(value) > 64 {
		value = value[:64]
	}
	return strings.TrimSpace(value)
}

// sendViaRelay delivers an alert to relay registration ids. Batching, the
// permanent-rejection cleanup handler and positional retry errors are shared
// with direct delivery through deliverAPNsBatch.
func (a *APNsAdapter) sendViaRelay(ctx context.Context, config map[string]any, payload map[string]any) error {
	if a.Relay == nil {
		return errors.New("apns: push relay is disabled (LABTETHER_PUSH_RELAY_URL)")
	}
	registrationIDs := extractDeviceTokens(config)
	if len(registrationIDs) == 0 {
		return nil
	}
	body, err := buildAPNsPayload(payload)
	if err != nil {
		return fmt.Errorf("apns: build payload: %w", err)
	}
	template := apnsRelayPush{
		PushType:   "alert",
		Priority:   apnsAlertPriority(payload),
		Expiration: payloadUnixTime(payload, "apns_expiration_unix"),
		CollapseID: relayCollapseID(apnsAlertCollapseID(payload)),
		Payload:    body,
	}
	return deliverAPNsBatch(ctx, config, registrationIDs, func(ctx context.Context, registrationID string) error {
		if !ValidRelayRegistrationID(registrationID) {
			return errors.New("push relay: stored registration id is malformed")
		}
		request := template
		request.RegistrationID = registrationID
		return a.Relay.push(ctx, request)
	})
}

// sendLiveActivityViaRelay delivers one ActivityKit update or end event. The
// relay picks the APNs host from the registration's own environment.
func (a *APNsAdapter) sendLiveActivityViaRelay(ctx context.Context, push LiveActivityPush) error {
	if a.Relay == nil {
		return errors.New("apns liveactivity: push relay is disabled (LABTETHER_PUSH_RELAY_URL)")
	}
	// Registration ids are case-sensitive: trim only, never lowercase.
	push.DeviceToken = strings.TrimSpace(push.DeviceToken)
	push.BundleID = strings.TrimSpace(push.BundleID)
	push.ActivityID = strings.TrimSpace(push.ActivityID)
	push.Event = strings.ToLower(strings.TrimSpace(push.Event))
	if push.BundleID != OfficialAppBundleID {
		return errors.New("apns liveactivity: the push relay serves only the official app")
	}
	if !ValidRelayRegistrationID(push.DeviceToken) {
		return errors.New("apns liveactivity: invalid relay registration")
	}
	if err := validateLiveActivityPushEvent(push); err != nil {
		return err
	}
	body, err := buildLiveActivityPayload(push)
	if err != nil {
		return fmt.Errorf("apns liveactivity: build payload: %w", err)
	}
	request := apnsRelayPush{
		RegistrationID: push.DeviceToken,
		PushType:       "liveactivity",
		Priority:       int64(liveActivityPriority(push.Priority)),
		CollapseID:     relayCollapseID(push.ActivityID),
		Payload:        body,
	}
	if !push.ExpiresAt.IsZero() && push.ExpiresAt.Unix() > 0 {
		request.Expiration = push.ExpiresAt.Unix()
	}
	return a.Relay.push(ctx, request)
}
