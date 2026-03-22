package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/updates"
)

const MaxJSONBodyBytes = 1 << 20 // 1MB limit for JSON request bodies

// DecodeJSONBody limits the request body size and decodes JSON into dst.
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		return err
	}

	// Reject payloads with trailing non-whitespace tokens to prevent ambiguous
	// parsing where valid JSON is followed by unexpected content.
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request body must contain a single JSON object")
	}
	return nil
}

func MapCommandLevel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success":
		return "info"
	case "queued", "running":
		return "debug"
	default:
		return "error"
	}
}

func ParseLimit(r *http.Request, fallback int) int {
	const maxLimit = 1000
	limit := fallback
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return limit
}

func ParseOffset(r *http.Request) int {
	s := r.URL.Query().Get("offset")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if n < 0 || err != nil {
		return 0
	}
	return n
}

func GroupIDQueryParam(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.URL.Query().Get("group_id"))
}

func ParseTimestampParam(raw string, fallback time.Time) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback.UTC()
	}

	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return fallback.UTC()
	}
	return parsed.UTC()
}

func ParseDurationParam(raw string, fallback, min, max time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	if parsed < min || parsed > max {
		return fallback
	}
	return parsed
}

func DefaultStepForWindow(window time.Duration) time.Duration {
	switch {
	case window <= time.Hour:
		return 15 * time.Second
	case window <= 6*time.Hour:
		return 30 * time.Second
	default:
		return time.Minute
	}
}

func RequestClientKey(r *http.Request) string {
	// Use RemoteAddr directly -- LabTether is accessed directly, not via reverse proxy.
	// X-Forwarded-For is trivially spoofable and must not be trusted for rate limiting.
	if r.RemoteAddr != "" {
		host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
		if err == nil && host != "" {
			return host
		}
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

func ValidateMaxLen(field, value string, maxLen int) error {
	if maxLen <= 0 {
		return nil
	}
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds max length %d", field, maxLen)
	}
	return nil
}

// ActionRunMatchesGroup returns true if the action run's target asset belongs
// to the specified group. assetGroup maps asset IDs to their group IDs.
func ActionRunMatchesGroup(run actions.Run, groupID string, assetGroup map[string]string) bool {
	if groupID == "" {
		return true
	}
	return assetGroup[run.Target] == groupID
}

// FilterLogEventsByGroup filters log events to only include those belonging to
// the specified group. It matches on both the event's group_id field and the
// asset's group assignment.
func FilterLogEventsByGroup(events []logs.Event, groupID string, assetGroup map[string]string) []logs.Event {
	if groupID == "" {
		return events
	}
	filtered := make([]logs.Event, 0, len(events))
	for _, event := range events {
		if event.Fields["group_id"] == groupID {
			filtered = append(filtered, event)
			continue
		}
		if assetGroup[event.AssetID] == groupID {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// GroupAssetIDsForGroup returns the asset IDs that belong to the specified group.
// assetGroup maps asset IDs to their group IDs.
func GroupAssetIDsForGroup(groupID string, assetGroup map[string]string) []string {
	if groupID == "" {
		return nil
	}
	var ids []string
	for assetID, gID := range assetGroup {
		if gID == groupID {
			ids = append(ids, assetID)
		}
	}
	return ids
}

// UpdatePlanTouchesGroup returns true if any of the plan's targets belong to the
// specified group.
func UpdatePlanTouchesGroup(plan updates.Plan, groupID string, assetGroup map[string]string) bool {
	if groupID == "" {
		return true
	}
	for _, target := range plan.Targets {
		if assetGroup[target] == groupID {
			return true
		}
	}
	return false
}
