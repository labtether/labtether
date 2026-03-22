package portainer

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/labtether/labtether/internal/assets"
)

// portainerResourceSummary holds running/stopped/total counts for containers or stacks.
type portainerResourceSummary struct {
	Running int `json:"running"`
	Stopped int `json:"stopped"`
	Total   int `json:"total"`
}

// portainerResourceCount holds a simple item count with an optional total size in bytes.
type portainerResourceCount struct {
	Count     int   `json:"count"`
	TotalSize int64 `json:"total_size,omitempty"`
}

// portainerOverviewResponse is the payload for the /portainer/assets/{id}/overview endpoint.
type portainerOverviewResponse struct {
	Containers      portainerResourceSummary `json:"containers"`
	Stacks          portainerResourceSummary `json:"stacks"`
	Images          portainerResourceCount   `json:"images"`
	Volumes         portainerResourceCount   `json:"volumes"`
	Networks        portainerResourceCount   `json:"networks"`
	ServerVersion   string                   `json:"server_version,omitempty"`
	DatabaseVersion string                   `json:"database_version,omitempty"`
	BuildNumber     string                   `json:"build_number,omitempty"`
	EndpointName    string                   `json:"endpoint_name,omitempty"`
	EndpointURL     string                   `json:"endpoint_url,omitempty"`
	Warnings        []string                 `json:"warnings,omitempty"`
}

// handlePortainerOverview returns a summary overview for a Portainer asset.
func (d *Deps) HandlePortainerOverview(ctx context.Context, w http.ResponseWriter, asset assets.Asset, runtime *PortainerRuntime) {
	endpointID, err := portainerEndpointID(asset)
	if err != nil {
		WritePortainerJSON(w, portainerOverviewResponse{
			Warnings: []string{err.Error()},
		}, nil)
		return
	}

	var warnings []string
	var resp portainerOverviewResponse

	// --- Containers ---
	containers, err := runtime.Client.GetContainers(ctx, endpointID)
	if err != nil {
		warnings = append(warnings, "containers unavailable: "+err.Error())
	} else {
		resp.Containers.Total = len(containers)
		for _, c := range containers {
			if c.State == "running" {
				resp.Containers.Running++
			} else {
				resp.Containers.Stopped++
			}
		}
	}

	// --- Stacks ---
	stacks, err := runtime.Client.GetStacks(ctx)
	if err != nil {
		warnings = append(warnings, "stacks unavailable: "+err.Error())
	} else {
		resp.Stacks.Total = len(stacks)
		for _, st := range stacks {
			// Stack.Status: 1 = active, 2 = inactive.
			if st.Status == 1 {
				resp.Stacks.Running++
			} else {
				resp.Stacks.Stopped++
			}
		}
	}

	// --- Images ---
	images, err := runtime.Client.GetImages(ctx, endpointID)
	if err != nil {
		warnings = append(warnings, "images unavailable: "+err.Error())
	} else {
		resp.Images.Count = len(images)
		// Sum up sizes from raw image JSON if available.
		for _, raw := range images {
			var img struct {
				Size int64 `json:"Size"`
			}
			if jerr := json.Unmarshal(raw, &img); jerr == nil {
				resp.Images.TotalSize += img.Size
			}
		}
	}

	// --- Volumes ---
	// GetVolumes returns the Docker API object: {"Volumes": [...], "Warnings": [...]}
	volumesRaw, err := runtime.Client.GetVolumes(ctx, endpointID)
	if err != nil {
		warnings = append(warnings, "volumes unavailable: "+err.Error())
	} else {
		var volWrapper struct {
			Volumes []json.RawMessage `json:"Volumes"`
		}
		if jerr := json.Unmarshal(volumesRaw, &volWrapper); jerr == nil {
			resp.Volumes.Count = len(volWrapper.Volumes)
		}
	}

	// --- Networks ---
	networks, err := runtime.Client.GetNetworks(ctx, endpointID)
	if err != nil {
		warnings = append(warnings, "networks unavailable: "+err.Error())
	} else {
		resp.Networks.Count = len(networks)
	}

	// --- Server metadata from asset.Metadata ---
	resp.ServerVersion = asset.Metadata["portainer_version"]
	resp.DatabaseVersion = asset.Metadata["portainer_database_version"]
	resp.BuildNumber = asset.Metadata["portainer_build_number"]
	resp.EndpointName = asset.Metadata["portainer_endpoint_name"]
	resp.EndpointURL = asset.Metadata["url"]

	WritePortainerJSON(w, resp, warnings)
}
