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
)

var (
	ErrCheckNotFound = errors.New("synthetic check not found")
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
