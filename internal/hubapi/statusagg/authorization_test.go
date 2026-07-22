package statusagg

import (
	"context"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/updates"
)

func statusTestContext(scopes, allowedAssets []string) context.Context {
	ctx := apiv2.ContextWithPrincipal(context.Background(), "apikey:key-test", "operator")
	ctx = apiv2.ContextWithScopes(ctx, scopes)
	ctx = apiv2.ContextWithAllowedAssets(ctx, allowedAssets)
	return ctx
}

func TestFilterStatusAssetsRequiresScopeAndAppliesAllowlist(t *testing.T) {
	assetList := []assets.Asset{{ID: "asset-a"}, {ID: "asset-b"}}

	withoutScope := filterStatusAssets(statusTestContext([]string{"hub:read"}, []string{"asset-a"}), assetList)
	if len(withoutScope) != 0 {
		t.Fatalf("hub:read must not implicitly expose assets, got %#v", withoutScope)
	}

	filtered := filterStatusAssets(statusTestContext([]string{"assets:read"}, []string{"asset-a"}), assetList)
	if len(filtered) != 1 || filtered[0].ID != "asset-a" {
		t.Fatalf("expected only asset-a, got %#v", filtered)
	}
}

func TestFilterStatusGroupsKeepsOnlyAccessibleGroupsAndAncestors(t *testing.T) {
	ctx := statusTestContext([]string{"groups:read", "assets:read"}, []string{"asset-a"})
	groupList := []groups.Group{
		{ID: "root"},
		{ID: "child", ParentGroupID: "root"},
		{ID: "unrelated"},
	}
	assetList := []assets.Asset{{ID: "asset-a", GroupID: "child"}}

	filtered := filterStatusGroups(ctx, groupList, assetList)
	if len(filtered) != 2 || filtered[0].ID != "root" || filtered[1].ID != "child" {
		t.Fatalf("expected child group and its ancestor only, got %#v", filtered)
	}
}

func TestFilterStatusUpdatePlansRejectsMixedTargetPlans(t *testing.T) {
	plans := []updates.Plan{
		{ID: "allowed", Targets: []string{"asset-a"}},
		{ID: "mixed", Targets: []string{"asset-a", "asset-b"}},
		{ID: "empty"},
	}
	allowedAssets := map[string]struct{}{"asset-a": {}}

	filtered, allowedPlanIDs := filterStatusUpdatePlans(plans, allowedAssets)
	if len(filtered) != 1 || filtered[0].ID != "allowed" {
		t.Fatalf("expected only the entirely allowed plan, got %#v", filtered)
	}
	if _, ok := allowedPlanIDs["allowed"]; !ok || len(allowedPlanIDs) != 1 {
		t.Fatalf("unexpected allowed plan IDs: %#v", allowedPlanIDs)
	}
}

func TestStatusScopeKeyChangesWhenAuthorizationChanges(t *testing.T) {
	base := statusTestContext([]string{"hub:read", "assets:read"}, []string{"asset-a"})
	changedScope := statusTestContext([]string{"hub:read"}, []string{"asset-a"})
	changedAssets := statusTestContext([]string{"hub:read", "assets:read"}, []string{"asset-b"})

	if statusScopeKey(base) == statusScopeKey(changedScope) {
		t.Fatal("scope changes must invalidate status cache isolation")
	}
	if statusScopeKey(base) == statusScopeKey(changedAssets) {
		t.Fatal("asset allowlist changes must invalidate status cache isolation")
	}
}

func TestStatusScopeKeySeparatesOwnerRoleFromSameActorAdminAndAPIKey(t *testing.T) {
	owner := apiv2.ContextWithPrincipal(context.Background(), "usr-same", "owner")
	admin := apiv2.ContextWithPrincipal(context.Background(), "usr-same", "admin")
	ownerKey := apiv2.ContextWithAPIKeyID(
		apiv2.ContextWithPrincipal(context.Background(), "usr-same", "owner"),
		"key-owner",
	)

	if statusScopeKey(owner) == statusScopeKey(admin) {
		t.Fatal("owner and admin authority must not share status cache entries")
	}
	if statusScopeKey(owner) == statusScopeKey(ownerKey) {
		t.Fatal("owner-role API key must not share interactive owner status cache entries")
	}
}
