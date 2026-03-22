package agentplatform

import (
	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentplatform/generic"
	"github.com/labtether/labtether/internal/platforms"
)

const defaultSource = "agent"

type providerFactory func(assetID, source string) agentcore.TelemetryProvider

// providerFactories is populated via per-OS init() calls in provider_<os>.go files.
var providerFactories = map[string]providerFactory{}

func DefaultSource() string {
	return defaultSource
}

// NewProvider returns the platform-specific telemetry provider for the current OS.
func NewProvider(assetID, source string) agentcore.TelemetryProvider {
	return NewProviderForPlatform(platforms.Current(), assetID, source)
}

// NewProviderForPlatform returns the platform-specific provider for the requested platform.
func NewProviderForPlatform(platform, assetID, source string) agentcore.TelemetryProvider {
	if factory, ok := providerFactories[platforms.Normalize(platform)]; ok {
		return factory(assetID, source)
	}
	return generic.New(assetID, source, "portable-helper")
}
