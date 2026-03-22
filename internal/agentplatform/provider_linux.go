//go:build linux

package agentplatform

import (
	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentplatform/linux"
	"github.com/labtether/labtether/internal/platforms"
)

func init() {
	providerFactories[platforms.Linux] = func(assetID, source string) agentcore.TelemetryProvider {
		return linux.New(assetID, source)
	}
}
