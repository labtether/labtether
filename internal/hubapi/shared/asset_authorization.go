package shared

import (
	"context"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
)

// HasAssetRestriction reports whether the authenticated principal is an API
// key with an explicit asset allowlist. Session principals and unrestricted
// API keys have a nil/empty allowlist and retain access to all assets.
func HasAssetRestriction(ctx context.Context) bool {
	return len(apiv2.AllowedAssetsFromContext(ctx)) > 0
}

// AllAssetsAllowed reports whether every supplied asset is in the request's
// allowlist. A restricted principal must supply at least one asset; this makes
// global or selector-based objects fail closed when their scope cannot be
// proven from concrete asset IDs.
func AllAssetsAllowed(ctx context.Context, assetIDs ...string) bool {
	allowed := apiv2.AllowedAssetsFromContext(ctx)
	if len(allowed) == 0 {
		return true
	}
	checked := 0
	for _, assetID := range assetIDs {
		assetID = strings.TrimSpace(assetID)
		if assetID == "" {
			continue
		}
		checked++
		if !apiv2.AssetCheck(allowed, assetID) {
			return false
		}
	}
	return checked > 0
}

// AccessibleGroupIDs returns the groups whose complete asset subtree is
// contained in the request's asset allowlist. Empty groups are intentionally
// omitted for restricted principals because they are global configuration
// objects with no concrete asset scope to authorize against.
func AccessibleGroupIDs(ctx context.Context, groupList []groups.Group, assetList []assets.Asset) map[string]struct{} {
	out := make(map[string]struct{}, len(groupList))
	if !HasAssetRestriction(ctx) {
		for _, groupEntry := range groupList {
			if groupID := strings.TrimSpace(groupEntry.ID); groupID != "" {
				out[groupID] = struct{}{}
			}
		}
		return out
	}

	parents := make(map[string]string, len(groupList))
	for _, groupEntry := range groupList {
		groupID := strings.TrimSpace(groupEntry.ID)
		if groupID != "" {
			parents[groupID] = strings.TrimSpace(groupEntry.ParentGroupID)
		}
	}

	total := make(map[string]int, len(groupList))
	permitted := make(map[string]int, len(groupList))
	for _, assetEntry := range assetList {
		groupID := strings.TrimSpace(assetEntry.GroupID)
		if groupID == "" {
			continue
		}
		assetAllowed := apiv2.AssetCheckContext(ctx, assetEntry.ID)
		visited := make(map[string]struct{}, 4)
		for groupID != "" {
			if _, seen := visited[groupID]; seen {
				break
			}
			visited[groupID] = struct{}{}
			total[groupID]++
			if assetAllowed {
				permitted[groupID]++
			}
			groupID = parents[groupID]
		}
	}

	for _, groupEntry := range groupList {
		groupID := strings.TrimSpace(groupEntry.ID)
		if groupID != "" && total[groupID] > 0 && total[groupID] == permitted[groupID] {
			out[groupID] = struct{}{}
		}
	}
	return out
}

// FilterGroupsByAssetAccess applies AccessibleGroupIDs while preserving the
// source ordering used by callers and persistence stores.
func FilterGroupsByAssetAccess(ctx context.Context, groupList []groups.Group, assetList []assets.Asset) []groups.Group {
	if !HasAssetRestriction(ctx) {
		return groupList
	}
	accessible := AccessibleGroupIDs(ctx, groupList, assetList)
	filtered := make([]groups.Group, 0, len(accessible))
	for _, groupEntry := range groupList {
		if _, ok := accessible[strings.TrimSpace(groupEntry.ID)]; ok {
			filtered = append(filtered, groupEntry)
		}
	}
	return filtered
}

// FilterAssetsByAccess returns only assets visible to the request principal.
func FilterAssetsByAccess(ctx context.Context, assetList []assets.Asset) []assets.Asset {
	if !HasAssetRestriction(ctx) {
		return assetList
	}
	filtered := make([]assets.Asset, 0, len(assetList))
	for _, assetEntry := range assetList {
		if apiv2.AssetCheckContext(ctx, assetEntry.ID) {
			filtered = append(filtered, assetEntry)
		}
	}
	return filtered
}

// AllCollectorAssetsAllowed reports whether a restricted principal can access
// every canonical asset owned by one connector collector. Provider APIs often
// expose collector-wide resources behind an asset-shaped URL; callers use this
// helper before such reads or mutations so an allowed parent asset cannot be
// used to reach a disallowed sibling. Legacy provider assets without a
// collector_id make the scope ambiguous and therefore fail closed.
func AllCollectorAssetsAllowed(ctx context.Context, store persistence.AssetStore, source, collectorID string) (bool, error) {
	if !HasAssetRestriction(ctx) {
		return true, nil
	}
	if store == nil {
		return false, fmt.Errorf("asset authorization store unavailable")
	}
	source = strings.TrimSpace(source)
	collectorID = strings.TrimSpace(collectorID)
	if source == "" || collectorID == "" {
		return false, nil
	}

	assetList, err := store.ListAssets()
	if err != nil {
		return false, err
	}
	found := false
	for _, assetEntry := range assetList {
		if !strings.EqualFold(strings.TrimSpace(assetEntry.Source), source) {
			continue
		}
		assetCollectorID := strings.TrimSpace(assetEntry.Metadata["collector_id"])
		if assetCollectorID == "" {
			return false, nil
		}
		if assetCollectorID != collectorID {
			continue
		}
		found = true
		if !apiv2.AssetCheckContext(ctx, assetEntry.ID) {
			return false, nil
		}
	}
	return found, nil
}
