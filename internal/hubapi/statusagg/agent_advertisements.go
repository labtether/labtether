package statusagg

import (
	"hash"
	"sort"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/assets"
)

// agentAdvertisements maps each agent device whose agent said its
// advertisement is complete, or named its edition, to what the Hub offers
// it. Devices left out keep today's pages and actions. nil when there are
// none, so the response is unchanged for such fleets.
func agentAdvertisements(assetList []assets.Asset) map[string]agentcaps.Advertisement {
	var out map[string]agentcaps.Advertisement
	for _, entry := range assetList {
		ad, ok := agentcaps.Describe(entry.Source, entry.Metadata)
		if !ok {
			continue
		}
		if out == nil {
			out = map[string]agentcaps.Advertisement{}
		}
		out[entry.ID] = ad
	}
	return out
}

func hashAgentAdvertisements(h hash.Hash, ads map[string]agentcaps.Advertisement) {
	ids := make([]string, 0, len(ads))
	for id := range ads {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		hashWriteString(h, id)
		hashWriteBool(h, ads[id].Explicit)
		hashWriteString(h, ads[id].Edition)
		for _, offered := range ads[id].Offered {
			hashWriteString(h, offered)
		}
	}
}
