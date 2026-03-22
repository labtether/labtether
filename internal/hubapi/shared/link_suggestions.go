package shared

import (
	"fmt"
	"log"
	"strings"

	"github.com/labtether/labtether/internal/discovery"
	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/persistence"
)

// LinkSuggestionDeps holds the stores required to run the link suggestion
// detector. Both fields must be non-nil for the detector to do any work.
type LinkSuggestionDeps struct {
	AssetStore persistence.AssetStore
	EdgeStore  edges.Store
}

// DetectLinkSuggestions runs the discovery engine over all known assets and
// creates or suggests edges in the edge store. It returns nil (without doing
// any work) when either store is nil or when fewer than two assets are present.
func (d *LinkSuggestionDeps) DetectLinkSuggestions() error {
	if d == nil || d.AssetStore == nil || d.EdgeStore == nil {
		return nil
	}

	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		return fmt.Errorf("list assets: %w", err)
	}
	if len(allAssets) < 2 {
		return nil
	}

	// Convert assets.Asset → discovery.AssetData.
	assetData := make([]discovery.AssetData, 0, len(allAssets))
	for _, a := range allAssets {
		assetData = append(assetData, discovery.AssetData{
			ID:       a.ID,
			Name:     a.Name,
			Source:   a.Source,
			Type:     a.Type,
			Host:     a.Host,
			Metadata: a.Metadata,
		})
	}

	engine := discovery.NewEngine(d.EdgeStore)
	created, suggested, err := engine.Run(assetData)
	if err != nil {
		return fmt.Errorf("discovery engine: %w", err)
	}
	if created > 0 || suggested > 0 {
		log.Printf("discovery engine: created %d auto edges, suggested %d edges", created, suggested)
	}

	return nil
}

// NormalizeMACAddress normalizes a MAC address to lowercase colon-separated
// form. It returns an empty string if the input is not a valid MAC address.
func NormalizeMACAddress(raw string) string {
	clean := strings.ToLower(strings.TrimSpace(raw))
	if clean == "" {
		return ""
	}
	// Replace common separators with colons.
	clean = strings.ReplaceAll(clean, "-", ":")
	clean = strings.ReplaceAll(clean, ".", ":")

	parts := strings.Split(clean, ":")
	// Handle dotted notation (e.g., "aabb.ccdd.eeff" -> 3 parts of 4 chars).
	if len(parts) == 3 && len(parts[0]) == 4 {
		expanded := make([]string, 0, 6)
		for _, part := range parts {
			if len(part) != 4 {
				return ""
			}
			expanded = append(expanded, part[:2], part[2:])
		}
		parts = expanded
	}
	if len(parts) != 6 {
		return ""
	}
	// Pad each part to 2 characters.
	for i, part := range parts {
		if len(part) == 1 {
			parts[i] = "0" + part
		}
		if len(parts[i]) != 2 {
			return ""
		}
		// Validate hex.
		for _, c := range parts[i] {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return ""
			}
		}
	}
	return strings.Join(parts, ":")
}
