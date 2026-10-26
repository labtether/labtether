package groupfeatures

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/persistence"
)

// EvaluateAssetGuardrailsContext keeps the asset, inherited host and maintenance
// reads on the caller's context. It never falls back to a legacy store method.
func (d *Deps) EvaluateAssetGuardrailsContext(ctx context.Context, assetID string, at time.Time) (GroupMaintenanceGuardrails, error) {
	if err := ctx.Err(); err != nil {
		return GroupMaintenanceGuardrails{}, err
	}
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return GroupMaintenanceGuardrails{}, nil
	}
	store, ok := d.AssetStore.(persistence.AssetContextStore)
	if !ok {
		return GroupMaintenanceGuardrails{}, fmt.Errorf("bounded asset maintenance lookup unavailable")
	}
	asset, found, err := store.GetAssetContext(ctx, assetID)
	if err != nil || !found {
		return GroupMaintenanceGuardrails{}, err
	}
	groupID := strings.TrimSpace(asset.GroupID)
	if groupID == "" {
		parentID := strings.TrimSpace(asset.Metadata["agent_id"])
		if parentID != "" && !strings.EqualFold(parentID, asset.ID) {
			parent, found, err := store.GetAssetContext(ctx, parentID)
			if err != nil {
				return GroupMaintenanceGuardrails{}, err
			}
			if found {
				groupID = strings.TrimSpace(parent.GroupID)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return GroupMaintenanceGuardrails{}, err
	}
	if groupID == "" {
		return GroupMaintenanceGuardrails{}, nil
	}
	return d.EvaluateGuardrailsContext(ctx, groupID, at)
}
