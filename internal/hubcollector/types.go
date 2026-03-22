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
)

var (
	ErrCollectorNotFound = errors.New("hub collector not found")
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
