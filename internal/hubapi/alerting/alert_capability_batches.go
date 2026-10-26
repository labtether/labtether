package alerting

import (
	"context"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

func (d *Deps) alertCapabilityBatchContext(ctx context.Context, candidates []assets.Asset, prefetched map[string][]string) (map[string][]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(candidates) > persistence.AlertCapabilityBatchSize {
		return nil, fmt.Errorf("alert capability candidate batch too large")
	}
	out := make(map[string][]string, len(candidates))
	missing := make([]string, 0, len(candidates))
	wanted := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		id := strings.TrimSpace(candidate.ID)
		if id == "" {
			return nil, fmt.Errorf("alert capability candidate has no ID")
		}
		if _, seen := wanted[id]; seen {
			continue
		}
		wanted[id] = struct{}{}
		// A supplied key is known, including an explicitly empty set. Missing
		// keys remain unknown and must never inherit global-list truncation.
		if known, found := prefetched[id]; found {
			out[id] = known
		} else {
			missing = append(missing, id)
			out[id] = nil
		}
	}
	if len(missing) == 0 || d.CanonicalStore == nil {
		return out, ctx.Err()
	}
	store, ok := d.CanonicalStore.(persistence.AlertCapabilityBatchStore)
	if !ok {
		return nil, fmt.Errorf("context-aware alert capability batch unavailable")
	}
	sets, err := store.GetResourceCapabilitySetsContext(ctx, missing)
	if err != nil {
		return nil, err
	}
	if len(sets) > len(missing) {
		return nil, fmt.Errorf("alert capability batch returned excess records")
	}
	remaining := make(map[string]struct{}, len(missing))
	for _, id := range missing {
		remaining[id] = struct{}{}
	}
	for _, set := range sets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, found := remaining[set.SubjectID]; !found || set.SubjectType != "resource" {
			return nil, fmt.Errorf("alert capability batch returned an unexpected identity")
		}
		delete(remaining, set.SubjectID)
		out[set.SubjectID] = d.capabilityIDsFromSet(set)
	}
	return out, ctx.Err()
}
