package persistence

import (
	"context"

	"github.com/labtether/labtether/internal/model"
)

func (m *MemoryCanonicalModelStore) GetResourceCapabilitySetsContext(ctx context.Context, ids []string) ([]model.CapabilitySet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ids, err := alertCapabilityBatchIDs(ids)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	if err := lockAlertEvaluation(ctx, &m.mu, true); err != nil {
		return nil, err
	}
	defer m.mu.RUnlock()
	out := make([]model.CapabilitySet, 0, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if set, found := m.capabilitySets[capabilitySetKey("resource", id)]; found {
			out = append(out, cloneCapabilitySet(set))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
