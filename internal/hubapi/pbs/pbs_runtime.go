package pbs

import (
	"time"

	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
)

// PBSRuntime holds a resolved PBS client and its metadata.
type PBSRuntime struct {
	Client      *pbsconnector.Client
	CollectorID string
	BaseURL     string
	TokenID     string
	SkipVerify  bool
	CaPEM       string
	Timeout     time.Duration
	ConfigKey   string
}

// CachedPBSRuntime wraps a runtime with its config key for cache invalidation.
type CachedPBSRuntime struct {
	Runtime   *PBSRuntime
	ConfigKey string
}
