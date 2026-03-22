package portainer

import (
	"time"

	pnconnector "github.com/labtether/labtether/internal/connectors/portainer"
)

// PortainerRuntime holds a resolved Portainer client and its metadata.
type PortainerRuntime struct {
	Client      *pnconnector.Client
	CollectorID string
	BaseURL     string
	AuthMethod  string
	SkipVerify  bool
	Timeout     time.Duration
	ConfigKey   string
}

// CachedPortainerRuntime wraps a runtime with its config key for cache invalidation.
type CachedPortainerRuntime struct {
	Runtime   *PortainerRuntime
	ConfigKey string
}
