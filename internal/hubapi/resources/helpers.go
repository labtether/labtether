package resources

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Thin aliases delegating to internal/hubapi/shared so that moved
// handler files keep compiling with minimal changes.

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	return shared.DecodeJSONBody(w, r, dst)
}

func validateMaxLen(field, value string, maxLen int) error {
	return shared.ValidateMaxLen(field, value, maxLen)
}

func parseLimit(r *http.Request, fallback int) int { return shared.ParseLimit(r, fallback) }

func parseDurationParam(raw string, fallback, min, max time.Duration) time.Duration {
	return shared.ParseDurationParam(raw, fallback, min, max)
}

func parseTimestampParam(raw string, fallback time.Time) time.Time {
	return shared.ParseTimestampParam(raw, fallback)
}
