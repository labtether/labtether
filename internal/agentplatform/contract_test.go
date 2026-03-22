package agentplatform

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/platforms"
)

func TestProviderContract(t *testing.T) {
	const (
		assetID = "contract-asset"
		source  = "contract-source"
	)

	provider := NewProvider(assetID, source)
	if provider == nil {
		t.Fatalf("expected provider")
	}

	info := provider.AgentInfo()
	if info.Status != "active" {
		t.Fatalf("expected active status, got %q", info.Status)
	}
	if info.OS == "" {
		t.Fatalf("expected OS value")
	}

	metadata := provider.StaticMetadata()
	if metadata["agent"] != source {
		t.Fatalf("expected metadata agent=%q, got %q", source, metadata["agent"])
	}
	if metadata["os"] == "" {
		t.Fatalf("expected metadata os value")
	}

	now := time.Now().UTC()
	sample, _ := provider.Collect(now)
	if sample.AssetID != assetID {
		t.Fatalf("expected sample asset_id=%q, got %q", assetID, sample.AssetID)
	}
	if sample.CollectedAt.IsZero() {
		t.Fatalf("expected collected_at value")
	}
	if sample.CPUPercent < 0 || sample.CPUPercent > 100 {
		t.Fatalf("cpu percent out of bounds: %.2f", sample.CPUPercent)
	}
	if sample.MemoryPercent < 0 || sample.MemoryPercent > 100 {
		t.Fatalf("memory percent out of bounds: %.2f", sample.MemoryPercent)
	}
	if sample.DiskPercent < 0 || sample.DiskPercent > 100 {
		t.Fatalf("disk percent out of bounds: %.2f", sample.DiskPercent)
	}
}

func TestDefaultSource(t *testing.T) {
	if got := DefaultSource(); got != "agent" {
		t.Fatalf("expected default source agent, got %q", got)
	}
}

func TestNewProviderForPlatformKnownPlatforms(t *testing.T) {
	const (
		assetID = "platform-contract-asset"
		source  = "platform-contract-source"
	)

	for _, platform := range platforms.Supported() {
		platform := platform
		t.Run(platform, func(t *testing.T) {
			provider := NewProviderForPlatform(platform, assetID, source)
			if provider == nil {
				t.Fatalf("expected provider for platform %s", platform)
			}

			info := provider.AgentInfo()
			if info.Status != "active" {
				t.Fatalf("expected active status, got %q", info.Status)
			}
		})
	}
}

func TestNewProviderForPlatformUnknownFallsBackToGeneric(t *testing.T) {
	provider := NewProviderForPlatform("openbsd", "fallback-asset", "fallback-source")
	if provider == nil {
		t.Fatalf("expected provider")
	}
	if mode := provider.AgentInfo().Mode; mode != "portable-helper" {
		t.Fatalf("expected portable-helper mode, got %q", mode)
	}
}
