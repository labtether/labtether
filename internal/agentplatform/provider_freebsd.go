//go:build freebsd

package agentplatform

import (
	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentplatform/freebsd"
	"github.com/labtether/labtether/internal/platforms"
)

func init() {
	providerFactories[platforms.FreeBSD] = func(assetID, source string) agentcore.TelemetryProvider {
		return freebsd.New(assetID, source)
	}
}
