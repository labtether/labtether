package hubcollector

import (
	"errors"
	"strings"
	"time"
)

const (
	CollectorTypeSSH           = "ssh"
	CollectorTypeWinRM         = "winrm"
	CollectorTypeAPI           = "api"
	CollectorTypeProxmox       = "proxmox"
	CollectorTypePBS           = "pbs"
	CollectorTypeTrueNAS       = "truenas"
	CollectorTypePortainer     = "portainer"
	CollectorTypeDocker        = "docker"
	CollectorTypeHomeAssistant = "homeassistant"

	DefaultIntervalSeconds = 60
	MinIntervalSeconds     = 1
	MaxIntervalSeconds     = 1<<31 - 1
)

var (
	ErrCollectorNotFound = errors.New("hub collector not found")
	ErrInvalidInterval   = errors.New("invalid hub collector interval_seconds")
)

type Collector struct {
	ID              string         `json:"id"`
	AssetID         string         `json:"asset_id"`
	CollectorType   string         `json:"collector_type"`
	Config          map[string]any `json:"config,omitempty"`
	Enabled         bool           `json:"enabled"`
	IntervalSeconds int            `json:"interval_seconds"`
	LastCollectedAt *time.Time     `json:"last_collected_at,omitempty"`
	LastStatus      string         `json:"last_status,omitempty"`
	LastError       string         `json:"last_error,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type CreateCollectorRequest struct {
	AssetID         string         `json:"asset_id"`
	CollectorType   string         `json:"collector_type"`
	Config          map[string]any `json:"config,omitempty"`
	Enabled         *bool          `json:"enabled,omitempty"`
	IntervalSeconds int            `json:"interval_seconds,omitempty"`
}

type UpdateCollectorRequest struct {
	Config          *map[string]any `json:"config,omitempty"`
	Enabled         *bool           `json:"enabled,omitempty"`
	IntervalSeconds *int            `json:"interval_seconds,omitempty"`
}

func NormalizeCollectorType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CollectorTypeSSH:
		return CollectorTypeSSH
	case CollectorTypeWinRM:
		return CollectorTypeWinRM
	case CollectorTypeAPI:
		return CollectorTypeAPI
	case CollectorTypeProxmox:
		return CollectorTypeProxmox
	case CollectorTypePBS:
		return CollectorTypePBS
	case CollectorTypeTrueNAS:
		return CollectorTypeTrueNAS
	case CollectorTypePortainer:
		return CollectorTypePortainer
	case CollectorTypeDocker:
		return CollectorTypeDocker
	case CollectorTypeHomeAssistant:
		return CollectorTypeHomeAssistant
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
