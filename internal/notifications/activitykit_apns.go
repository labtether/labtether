package notifications

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const liveActivityTopicSuffix = ".push-type.liveactivity"

// LiveActivityContentState mirrors the Codable state used by the iOS widget.
// Dates are encoded using Foundation's default Codable representation: seconds
// since 2001-01-01 00:00:00 UTC.
type LiveActivityContentState struct {
	Title           string
	Summary         string
	Status          string
	Severity        string
	Assignee        string
	StartedAt       time.Time
	UpdatedAt       time.Time
	ShowFullDetails bool
	CanMutate       bool
}

// LiveActivityPush is one update or end event for an already-created Activity.
type LiveActivityPush struct {
	DeviceToken string
	BundleID    string
	ActivityID  string
	Event       string
	State       LiveActivityContentState
	Timestamp   time.Time
	StaleAt     *time.Time
	DismissAt   *time.Time
	ExpiresAt   time.Time
	Priority    int
}

// SendLiveActivity delivers an ActivityKit-specific APNs request using the
// same credential/JWT cache and bounded HTTP client as normal notification
// delivery. It intentionally does not route through the alert payload builder.
func (a *APNsAdapter) SendLiveActivity(
	ctx context.Context,
	config map[string]any,
	push LiveActivityPush,
) error {
	authKeyPath, _ := config["auth_key_path"].(string)
	keyID, _ := config["key_id"].(string)
	teamID, _ := config["team_id"].(string)
	configuredBundleID, _ := config["bundle_id"].(string)
	authKeyPath = strings.TrimSpace(authKeyPath)
	keyID = strings.TrimSpace(keyID)
	teamID = strings.TrimSpace(teamID)
	configuredBundleID = strings.TrimSpace(configuredBundleID)
	push.BundleID = strings.TrimSpace(push.BundleID)
	push.DeviceToken = strings.ToLower(strings.TrimSpace(push.DeviceToken))
	push.ActivityID = strings.TrimSpace(push.ActivityID)
	push.Event = strings.ToLower(strings.TrimSpace(push.Event))

	if authKeyPath == "" || keyID == "" || teamID == "" || configuredBundleID == "" {
		return fmt.Errorf("apns liveactivity: incomplete APNs configuration")
	}
	if configuredBundleID != push.BundleID {
		return fmt.Errorf("apns liveactivity: registered bundle does not match channel topic")
	}
	if !validLiveActivityPushToken(push.DeviceToken) {
		return fmt.Errorf("apns liveactivity: invalid device token")
	}
	if push.ActivityID == "" {
		return fmt.Errorf("apns liveactivity: activity id is required")
	}
	if push.Event != "update" && push.Event != "end" {
		return fmt.Errorf("apns liveactivity: event must be update or end")
	}

	jwt, err := a.authorizationJWT(authKeyPath, keyID, teamID)
	if err != nil {
		return err
	}
	body, err := buildLiveActivityPayload(push)
	if err != nil {
		return fmt.Errorf("apns liveactivity: build payload: %w", err)
	}

	endpoint := apnsSandboxEndpoint
	if production, _ := config["production"].(bool); production {
		endpoint = apnsProductionEndpoint
	}
	requestURL := fmt.Sprintf("%s/3/device/%s", endpoint, url.PathEscape(push.DeviceToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("apns liveactivity: create request: %w", err)
	}
	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", configuredBundleID+liveActivityTopicSuffix)
	req.Header.Set("apns-push-type", "liveactivity")
	priority := push.Priority
	if priority != 5 && priority != 10 {
		priority = 10
	}
	req.Header.Set("apns-priority", strconv.Itoa(priority))
	req.Header.Set("apns-collapse-id", boundedHeaderValue(push.ActivityID, 64))
	if !push.ExpiresAt.IsZero() {
		req.Header.Set("apns-expiration", strconv.FormatInt(push.ExpiresAt.Unix(), 10))
	}

	resp, err := sharedAPNsHTTPClient.Do(req) // #nosec G704 -- APNs host is selected from fixed Apple endpoints.
	if err != nil {
		return fmt.Errorf("apns liveactivity: request failed: %s", sanitizedAPNSTransportError(err, push.DeviceToken))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return newAPNsResponseError(resp.StatusCode, responseBody)
}

// IsPermanentAPNsTokenRejection reports whether APNs has proven that a token
// can no longer be used for its topic. Callers may safely remove that exact
// encrypted registration instead of retrying it.
func IsPermanentAPNsTokenRejection(err error) bool {
	return isPermanentAPNsTokenRejection(err)
}

// ValidLiveActivityPushToken validates the canonical ActivityKit token wire
// format without returning or logging any portion of the credential.
func ValidLiveActivityPushToken(token string) bool {
	return validLiveActivityPushToken(strings.ToLower(strings.TrimSpace(token)))
}

func validLiveActivityPushToken(token string) bool {
	// ActivityKit exposes this credential as opaque Data, and Apple does not
	// contractually fix APNs token length. Accept a bounded, nonempty even-length
	// hex representation while rejecting path/control syntax. The 100-byte cap
	// comfortably covers current tokens without allowing unbounded URLs/storage.
	if len(token) < 2 || len(token) > 200 || len(token)%2 != 0 {
		return false
	}
	decoded, err := hex.DecodeString(token)
	return err == nil && len(decoded) > 0 && len(decoded) <= 100
}

func buildLiveActivityPayload(push LiveActivityPush) ([]byte, error) {
	timestamp := push.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	state := map[string]any{
		"title":           boundedUTF8(push.State.Title, 160),
		"status":          boundedUTF8(push.State.Status, 32),
		"severity":        boundedUTF8(push.State.Severity, 16),
		"startedAt":       foundationDateValue(push.State.StartedAt),
		"updatedAt":       foundationDateValue(push.State.UpdatedAt),
		"showFullDetails": push.State.ShowFullDetails,
		"canMutate":       push.State.CanMutate,
	}
	if summary := boundedUTF8(push.State.Summary, 512); summary != "" {
		state["summary"] = summary
	}
	if assignee := boundedUTF8(push.State.Assignee, 96); assignee != "" {
		state["assignee"] = assignee
	}

	aps := map[string]any{
		"timestamp":     timestamp.Unix(),
		"event":         push.Event,
		"content-state": state,
	}
	if push.Event == "update" && push.StaleAt != nil {
		aps["stale-date"] = push.StaleAt.Unix()
	}
	if push.Event == "end" && push.DismissAt != nil {
		aps["dismissal-date"] = push.DismissAt.Unix()
	}
	encoded, err := json.Marshal(map[string]any{"aps": aps})
	if err != nil {
		return nil, err
	}
	if len(encoded) > 4*1024 {
		return nil, fmt.Errorf("payload exceeds APNs 4 KB limit")
	}
	return encoded, nil
}

func foundationDateValue(value time.Time) float64 {
	if value.IsZero() {
		return 0
	}
	return float64(value.UnixNano())/float64(time.Second) - 978307200
}
