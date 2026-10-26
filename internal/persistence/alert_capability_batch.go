package persistence

import (
	"context"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/model"
)

const AlertCapabilityBatchSize = 128

// AlertCapabilityBatchStore reads every existing resource set for these exact
// IDs in one owner snapshot. A successful missing row means absent, not omitted
// by a global listing limit. Callers consume one batch at a time; record payload
// sizes remain variable. Errors must not return a usable partial result.
type AlertCapabilityBatchStore interface {
	GetResourceCapabilitySetsContext(context.Context, []string) ([]model.CapabilitySet, error)
}

func alertCapabilityBatchIDs(ids []string) ([]string, error) {
	if len(ids) > AlertCapabilityBatchSize {
		return nil, fmt.Errorf("alert capability batch exceeds %d IDs", AlertCapabilityBatchSize)
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("alert capability batch contains an empty ID")
		}
		if _, found := seen[id]; !found {
			out = append(out, id)
			seen[id] = struct{}{}
		}
	}
	return out, nil
}
