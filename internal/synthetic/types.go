package synthetic

import (
	"errors"
	"strings"
	"time"
)

const (
	CheckTypeHTTP    = "http"
	CheckTypeTCP     = "tcp"
	CheckTypeDNS     = "dns"
	CheckTypeTLSCert = "tls_cert"

	ResultStatusOK      = "ok"
	ResultStatusFail    = "fail"
	ResultStatusTimeout = "timeout"

	DefaultIntervalSeconds = 60
	MinIntervalSeconds     = 1
	MaxIntervalSeconds     = 1<<31 - 1
)

var (
	ErrCheckNotFound   = errors.New("synthetic check not found")
	ErrInvalidInterval = errors.New("invalid synthetic interval_seconds")
)

type Check struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	CheckType       string         `json:"check_type"`
	Target          string         `json:"target"`
	Config          map[string]any `json:"config,omitempty"`
	IntervalSeconds int            `json:"interval_seconds"`
	Enabled         bool           `json:"enabled"`
	LastRunAt       *time.Time     `json:"last_run_at,omitempty"`
	LastStatus      string         `json:"last_status,omitempty"`
	ServiceID       string         `json:"service_id,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type CreateCheckRequest struct {
	Name            string         `json:"name"`
	CheckType       string         `json:"check_type"`
	Target          string         `json:"target"`
	Config          map[string]any `json:"config,omitempty"`
	IntervalSeconds int            `json:"interval_seconds,omitempty"`
	Enabled         *bool          `json:"enabled,omitempty"`
	ServiceID       string         `json:"service_id,omitempty"`
}

type UpdateCheckRequest struct {
	Name            *string         `json:"name,omitempty"`
	Target          *string         `json:"target,omitempty"`
	Config          *map[string]any `json:"config,omitempty"`
	IntervalSeconds *int            `json:"interval_seconds,omitempty"`
	Enabled         *bool           `json:"enabled,omitempty"`
}

type Result struct {
	ID        string         `json:"id"`
	CheckID   string         `json:"check_id"`
	Status    string         `json:"status"`
	LatencyMS *int           `json:"latency_ms,omitempty"`
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CheckedAt time.Time      `json:"checked_at"`
}

func NormalizeCheckType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CheckTypeHTTP:
		return CheckTypeHTTP
	case CheckTypeTCP:
		return CheckTypeTCP
	case CheckTypeDNS:
		return CheckTypeDNS
	case CheckTypeTLSCert:
		return CheckTypeTLSCert
	default:
		return ""
	}
}

func NormalizeResultStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ResultStatusOK:
		return ResultStatusOK
	case ResultStatusFail:
		return ResultStatusFail
	case ResultStatusTimeout:
		return ResultStatusTimeout
	default:
		return ""
	}
}

func ValidateIntervalSeconds(value int) error {
	if value < MinIntervalSeconds || value > MaxIntervalSeconds {
		return ErrInvalidInterval
	}
	return nil
}

func ValidateCreateIntervalSeconds(value int) error {
	if value == 0 {
		return nil
	}
	return ValidateIntervalSeconds(value)
}

func CreateIntervalSeconds(value int) (int, error) {
	if value == 0 {
		return DefaultIntervalSeconds, nil
	}
	if err := ValidateIntervalSeconds(value); err != nil {
		return 0, err
	}
	return value, nil
}

func IntervalDuration(value int) time.Duration {
	if ValidateIntervalSeconds(value) != nil {
		return 0
	}
	return time.Duration(value) * time.Second
}
