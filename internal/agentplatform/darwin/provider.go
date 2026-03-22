package darwin

import (
	"time"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentplatform/generic"
)

type Provider struct {
	base           *generic.Provider
	staticMetadata map[string]string
}

func New(assetID, source string) *Provider {
	base := generic.New(assetID, source, "launchd-helper")
	metadata := base.StaticMetadata()
	for key, value := range readTailscaleMetadata() {
		metadata[key] = value
	}
	for key, value := range readCapabilityMetadata() {
		metadata[key] = value
	}

	return &Provider{
		base:           base,
		staticMetadata: metadata,
	}
}

func (p *Provider) AgentInfo() agentcore.AgentInfo {
	return p.base.AgentInfo()
}

func (p *Provider) StaticMetadata() map[string]string {
	out := make(map[string]string, len(p.staticMetadata))
	for key, value := range p.staticMetadata {
		out[key] = value
	}
	return out
}

func (p *Provider) Collect(now time.Time) (agentcore.TelemetrySample, error) {
	return p.base.Collect(now)
}
