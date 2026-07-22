package shared

import (
	"context"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
)

func TestAccessibleGroupIDsRequiresCompleteSubtree(t *testing.T) {
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	groupList := []groups.Group{
		{ID: "root"},
		{ID: "allowed-child", ParentGroupID: "root"},
		{ID: "secret-child", ParentGroupID: "root"},
		{ID: "empty"},
	}
	assetList := []assets.Asset{
		{ID: "asset-a", GroupID: "allowed-child"},
		{ID: "asset-b", GroupID: "secret-child"},
	}

	accessible := AccessibleGroupIDs(ctx, groupList, assetList)
	if _, ok := accessible["allowed-child"]; !ok {
		t.Fatal("expected fully allowed child group to be accessible")
	}
	for _, forbidden := range []string{"root", "secret-child", "empty"} {
		if _, ok := accessible[forbidden]; ok {
			t.Fatalf("did not expect %q to be accessible", forbidden)
		}
	}
}

func TestAllAssetsAllowedFailsClosedWithoutConcreteTargets(t *testing.T) {
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	if AllAssetsAllowed(ctx) {
		t.Fatal("restricted principal without concrete targets must fail closed")
	}
	if AllAssetsAllowed(ctx, "asset-a", "asset-b") {
		t.Fatal("mixed allowed and forbidden targets must fail closed")
	}
	if !AllAssetsAllowed(ctx, "asset-a") {
		t.Fatal("allowed target should pass")
	}
}

func TestAllCollectorAssetsAllowedRequiresCompleteCollector(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	for _, req := range []assets.HeartbeatRequest{
		{AssetID: "allowed", Source: "truenas", Metadata: map[string]string{"collector_id": "tn-1"}},
		{AssetID: "denied", Source: "truenas", Metadata: map[string]string{"collector_id": "tn-1"}},
		{AssetID: "other", Source: "truenas", Metadata: map[string]string{"collector_id": "tn-2"}},
	} {
		if _, err := store.UpsertAssetHeartbeat(req); err != nil {
			t.Fatal(err)
		}
	}

	partial := apiv2.ContextWithAllowedAssets(context.Background(), []string{"allowed"})
	allowed, err := AllCollectorAssetsAllowed(partial, store, "truenas", "tn-1")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("partial collector access must fail closed")
	}

	complete := apiv2.ContextWithAllowedAssets(context.Background(), []string{"allowed", "denied"})
	allowed, err = AllCollectorAssetsAllowed(complete, store, "truenas", "tn-1")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("complete access to the selected collector should pass")
	}
}

func TestAllCollectorAssetsAllowedRejectsAmbiguousLegacyProviderAsset(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	for _, req := range []assets.HeartbeatRequest{
		{AssetID: "allowed", Source: "pbs", Metadata: map[string]string{"collector_id": "pbs-1"}},
		{AssetID: "legacy", Source: "pbs"},
	} {
		if _, err := store.UpsertAssetHeartbeat(req); err != nil {
			t.Fatal(err)
		}
	}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"allowed", "legacy"})
	allowed, err := AllCollectorAssetsAllowed(ctx, store, "pbs", "pbs-1")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("provider asset without collector mapping must fail closed")
	}
}
