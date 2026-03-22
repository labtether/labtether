package portainer

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

// portainerEndpointSummary is the API representation of a Portainer endpoint (container-host).
type portainerEndpointSummary struct {
	AssetID          string    `json:"asset_id"`
	EndpointID       string    `json:"endpoint_id,omitempty"`
	Name             string    `json:"name"`
	NormalizedID     string    `json:"normalized_id"`
	URL              string    `json:"url,omitempty"`
	PortainerVersion string    `json:"portainer_version,omitempty"`
	EngineOS         string    `json:"engine_os,omitempty"`
	EngineArch       string    `json:"engine_arch,omitempty"`
	ContainerCount   int       `json:"container_count"`
	StackCount       int       `json:"stack_count"`
	ImageCount       int       `json:"image_count"`
	LastSeen         time.Time `json:"last_seen"`
	Source           string    `json:"source"`
}

// HandlePortainerEndpoints handles GET /portainer/endpoints — list all Portainer endpoints.
func (d *Deps) HandlePortainerEndpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.AssetStore == nil {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"endpoints": []any{}})
		return
	}

	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list assets")
		return
	}

	// Separate portainer assets by type for counting.
	var endpoints []assets.Asset
	containersByEndpoint := make(map[string]int)
	stacksByEndpoint := make(map[string]int)

	for _, a := range allAssets {
		if a.Source != "portainer" {
			continue
		}
		endpointID := strings.TrimSpace(a.Metadata["endpoint_id"])
		switch a.Type {
		case "container-host":
			endpoints = append(endpoints, a)
		case "container":
			if endpointID != "" {
				containersByEndpoint[endpointID]++
			}
		case "stack", "compose-stack":
			if endpointID != "" {
				stacksByEndpoint[endpointID]++
			}
		}
	}

	summaries := make([]portainerEndpointSummary, 0, len(endpoints))
	for _, ep := range endpoints {
		epID := strings.TrimSpace(ep.Metadata["endpoint_id"])
		containerCount := containersByEndpoint[epID]
		stackCount := stacksByEndpoint[epID]

		// Use pre-computed counts from metadata if live counts are zero.
		if containerCount == 0 {
			if v, err := strconv.Atoi(strings.TrimSpace(ep.Metadata["portainer_container_count"])); err == nil {
				containerCount = v
			}
		}
		if stackCount == 0 {
			if v, err := strconv.Atoi(strings.TrimSpace(ep.Metadata["portainer_stack_count"])); err == nil {
				stackCount = v
			}
		}

		summaries = append(summaries, portainerEndpointSummary{
			AssetID:          ep.ID,
			EndpointID:       epID,
			Name:             ep.Name,
			NormalizedID:     normalizePortainerEndpointID(ep.ID),
			URL:              strings.TrimSpace(ep.Metadata["url"]),
			PortainerVersion: strings.TrimSpace(ep.Metadata["portainer_version"]),
			EngineOS:         strings.TrimSpace(ep.Metadata["engine_os"]),
			EngineArch:       strings.TrimSpace(ep.Metadata["engine_arch"]),
			ContainerCount:   containerCount,
			StackCount:       stackCount,
			ImageCount:       0, // images are not tracked as separate assets
			LastSeen:         ep.LastSeenAt,
			Source:           "portainer",
		})
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"endpoints": summaries})
}

// normalizePortainerEndpointID produces a lowercase, dash-separated lookup key
// from an asset ID, mirroring the Docker host normalization pattern.
func normalizePortainerEndpointID(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = strings.ReplaceAll(normalized, ".", "-")
	return normalized
}
