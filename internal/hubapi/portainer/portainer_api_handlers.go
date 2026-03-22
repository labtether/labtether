package portainer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

// portainerResponse wraps Portainer API data with metadata.
type PortainerResponse struct {
	Data      any       `json:"data"`
	FetchedAt time.Time `json:"fetched_at"`
	Warnings  []string  `json:"warnings,omitempty"`
}

// writePortainerJSON wraps data in a portainerResponse and writes it as JSON.
func WritePortainerJSON(w http.ResponseWriter, data any, warnings []string) {
	resp := PortainerResponse{
		Data:      data,
		FetchedAt: time.Now().UTC(),
		Warnings:  DedupeNonEmptyWarnings(warnings),
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
}

// handlePortainerAssets dispatches /portainer/assets/{assetID}/{action}[/...].
func (d *Deps) HandlePortainerAssets(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/portainer/assets/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "portainer asset path not found")
		return
	}
	parts := strings.Split(path, "/")
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "portainer asset path not found")
		return
	}
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown portainer asset action")
		return
	}
	action := strings.TrimSpace(parts[1])
	subParts := parts[2:]

	switch action {
	case "capabilities":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		asset, err := d.ResolvePortainerAsset(assetID)
		if err != nil {
			writePortainerResolveError(w, err)
			return
		}
		d.HandlePortainerCapabilities(w, asset)
		return
	case "overview":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	case "containers", "stacks", "images", "volumes", "networks":
		// method checking is delegated to the sub-handlers; exec is WebSocket
		// and handled before the timeout context is created below.
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown portainer asset action")
		return
	}

	// Exec sessions are long-lived WebSocket connections; dispatch before
	// applying the short-lived context timeout used by regular API calls.
	// Auth is already enforced by the withAuth middleware at route registration.
	if action == "containers" && len(subParts) == 2 && strings.TrimSpace(subParts[1]) == "exec" {
		containerID := strings.TrimSpace(subParts[0])
		if containerID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "container id required")
			return
		}
		d.HandlePortainerContainerExec(w, r, assetID, containerID)
		return
	}

	asset, runtime, err := d.ResolvePortainerAssetRuntime(assetID)
	if err != nil {
		writePortainerResolveError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch action {
	case "overview":
		d.HandlePortainerOverview(ctx, w, asset, runtime)
	case "containers":
		d.HandlePortainerContainers(ctx, w, r, asset, runtime, subParts)
	case "stacks":
		d.HandlePortainerStacks(ctx, w, r, asset, runtime, subParts)
	case "images":
		d.HandlePortainerImages(ctx, w, r, asset, runtime, subParts)
	case "volumes":
		d.HandlePortainerVolumes(ctx, w, r, asset, runtime, subParts)
	case "networks":
		d.HandlePortainerNetworks(ctx, w, r, asset, runtime, subParts)
	}
}

// portainerEndpointID extracts the endpoint_id from asset metadata and converts it to int.
func portainerEndpointID(asset assets.Asset) (int, error) {
	raw := strings.TrimSpace(asset.Metadata["endpoint_id"])
	if raw == "" {
		return 0, fmt.Errorf("asset metadata missing endpoint_id")
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid endpoint_id %q: %w", raw, err)
	}
	return id, nil
}
