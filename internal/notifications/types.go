package notifications

import (
	"errors"
	"strings"
	"time"
)

const (
	ChannelTypeWebhook = "webhook"
	ChannelTypeEmail   = "email"
	ChannelTypeSlack   = "slack"
	ChannelTypeAPNs    = "apns"
	ChannelTypeNtfy    = "ntfy"
	ChannelTypeGotify  = "gotify"

	RecordStatusPending = "pending"
	RecordStatusSent    = "sent"
	RecordStatusFailed  = "failed"

	DefaultMaxRetries = 3
)

var (
	ErrChannelNotFound = errors.New("notification channel not found")
	ErrRouteNotFound   = errors.New("alert route not found")
)

type Channel struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Config    map[string]any `json:"config,omitempty"`
	Enabled   bool           `json:"enabled"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type CreateChannelRequest struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Config  map[string]any `json:"config,omitempty"`
	Enabled *bool          `json:"enabled,omitempty"`
}

type UpdateChannelRequest struct {
	Name    *string         `json:"name,omitempty"`
	Config  *map[string]any `json:"config,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`
}

type Route struct {
	ID                    string         `json:"id"`
	Name                  string         `json:"name"`
	Matchers              map[string]any `json:"matchers,omitempty"`
	ChannelIDs            []string       `json:"channel_ids,omitempty"`
	SeverityFilter        string         `json:"severity_filter,omitempty"`
	GroupFilter           string         `json:"group_filter,omitempty"`
	GroupBy               []string       `json:"group_by,omitempty"`
	GroupWaitSeconds      int            `json:"group_wait_seconds"`
	GroupIntervalSeconds  int            `json:"group_interval_seconds"`
	RepeatIntervalSeconds int            `json:"repeat_interval_seconds"`
	Enabled               bool           `json:"enabled"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

type CreateRouteRequest struct {
	Name                  string         `json:"name"`
	Matchers              map[string]any `json:"matchers,omitempty"`
	ChannelIDs            []string       `json:"channel_ids,omitempty"`
	SeverityFilter        string         `json:"severity_filter,omitempty"`
	GroupFilter           string         `json:"group_filter,omitempty"`
	GroupBy               []string       `json:"group_by,omitempty"`
	GroupWaitSeconds      int            `json:"group_wait_seconds,omitempty"`
	GroupIntervalSeconds  int            `json:"group_interval_seconds,omitempty"`
	RepeatIntervalSeconds int            `json:"repeat_interval_seconds,omitempty"`
	Enabled               *bool          `json:"enabled,omitempty"`
}

type UpdateRouteRequest struct {
	Name                  *string         `json:"name,omitempty"`
	Matchers              *map[string]any `json:"matchers,omitempty"`
	ChannelIDs            *[]string       `json:"channel_ids,omitempty"`
	SeverityFilter        *string         `json:"severity_filter,omitempty"`
	GroupFilter           *string         `json:"group_filter,omitempty"`
	GroupBy               *[]string       `json:"group_by,omitempty"`
	GroupWaitSeconds      *int            `json:"group_wait_seconds,omitempty"`
	GroupIntervalSeconds  *int            `json:"group_interval_seconds,omitempty"`
	RepeatIntervalSeconds *int            `json:"repeat_interval_seconds,omitempty"`
	Enabled               *bool           `json:"enabled,omitempty"`
}

type Record struct {
	ID              string     `json:"id"`
	ChannelID       string     `json:"channel_id"`
	AlertInstanceID string     `json:"alert_instance_id,omitempty"`
	RouteID         string     `json:"route_id,omitempty"`
	Status          string     `json:"status"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
	Error           string     `json:"error,omitempty"`
	RetryCount      int        `json:"retry_count"`
	MaxRetries      int        `json:"max_retries"`
	NextRetryAt     *time.Time `json:"next_retry_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// RetryBackoff returns the exponential backoff duration for the given retry attempt.
// Attempt 0 → 30s, attempt 1 → 60s, attempt 2 → 120s, capped at 10 minutes.
func RetryBackoff(attempt int) time.Duration {
	base := 30 * time.Second
	if attempt <= 0 {
		return base
	}
	if attempt >= 5 {
		return 10 * time.Minute
	}
	d := base * time.Duration(1<<attempt)
	if d > 10*time.Minute {
		d = 10 * time.Minute
	}
	return d
}

type CreateRecordRequest struct {
	ChannelID       string `json:"channel_id"`
	AlertInstanceID string `json:"alert_instance_id,omitempty"`
	RouteID         string `json:"route_id,omitempty"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
}

func NormalizeChannelType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ChannelTypeWebhook:
		return ChannelTypeWebhook
	case ChannelTypeEmail:
		return ChannelTypeEmail
	case ChannelTypeSlack:
		return ChannelTypeSlack
	case ChannelTypeAPNs:
		return ChannelTypeAPNs
	case ChannelTypeNtfy:
		return ChannelTypeNtfy
	case ChannelTypeGotify:
		return ChannelTypeGotify
	default:
		return ""
	}
}

func NormalizeRecordStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case RecordStatusPending:
		return RecordStatusPending
	case RecordStatusSent:
		return RecordStatusSent
	case RecordStatusFailed:
		return RecordStatusFailed
	default:
		return ""
	}
}
