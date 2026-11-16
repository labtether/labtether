package resources

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/persistence"
)

// TestPersistCanonicalHeartbeatUsesCommittedSource catches the writer taking
// the heartbeat's own source before the committed asset's: an agent that
// reports "proxmox" would get a Proxmox connector provider and binding.
func TestPersistCanonicalHeartbeatUsesCommittedSource(t *testing.T) {
	t.Parallel()
	store := persistence.NewMemoryCanonicalModelStore()
	assetID := "canonical-source-authority"
	now := time.Now().UTC()
	committed := assets.Asset{
		ID: assetID, Type: "host", Name: assetID, Source: "agent", Status: "online",
		Platform: "linux", LastSeenAt: now, UpdatedAt: now, CreatedAt: now,
	}
	PersistCanonicalHeartbeatForAsset(store, committed, assets.HeartbeatRequest{
		AssetID: assetID, Source: "proxmox",
	}, now)
	agentProviderID := CanonicalProviderInstanceID(model.ProviderKindAgent, "agent", assetID)
	if _, ok, err := store.GetProviderInstance(agentProviderID); err != nil || !ok {
		t.Fatalf("committed agent provider missing ok=%v err=%v", ok, err)
	}
	forgedProviderID := CanonicalProviderInstanceID(model.ProviderKindConnector, "proxmox", assetID)
	if _, ok, err := store.GetProviderInstance(forgedProviderID); err != nil || ok {
		t.Fatalf("payload-selected provider exists=%v err=%v", ok, err)
	}
}
