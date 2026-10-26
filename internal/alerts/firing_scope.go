package alerts

import (
	"fmt"
	"sort"
	"strings"
)

// FiringScope records positive identities witnessed by the condition read that
// first fired an instance. Empty lists do not mean global scope. NULL on the
// instance means the firing predates this evidence or its origin is unknown.
type FiringScope struct {
	Version           int      `json:"version"`
	AssetIDs          []string `json:"asset_ids,omitempty"`
	GroupIDs          []string `json:"group_ids,omitempty"`
	SyntheticCheckIDs []string `json:"synthetic_check_ids,omitempty"`
}

func NormalizeFiringScope(input FiringScope) (FiringScope, error) {
	if input.Version != 1 {
		return FiringScope{}, fmt.Errorf("unknown alert firing scope version")
	}
	out := FiringScope{Version: 1}
	for _, pair := range []struct {
		in  []string
		out *[]string
	}{
		{input.AssetIDs, &out.AssetIDs}, {input.GroupIDs, &out.GroupIDs}, {input.SyntheticCheckIDs, &out.SyntheticCheckIDs},
	} {
		seen := map[string]bool{}
		for _, id := range pair.in {
			if id == "" || strings.TrimSpace(id) != id {
				return FiringScope{}, fmt.Errorf("invalid alert firing scope identity")
			}
			if !seen[id] {
				*pair.out = append(*pair.out, id)
				seen[id] = true
			}
		}
		sort.Strings(*pair.out)
	}
	return out, nil
}

func CloneFiringScope(input *FiringScope) *FiringScope {
	if input == nil {
		return nil
	}
	out := *input
	out.AssetIDs = append([]string(nil), input.AssetIDs...)
	out.GroupIDs = append([]string(nil), input.GroupIDs...)
	out.SyntheticCheckIDs = append([]string(nil), input.SyntheticCheckIDs...)
	return &out
}
