package portainer

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

// handlePortainerImages dispatches image-related sub-routes for a Portainer asset.
//
// Routes (subParts is parts[2:] after assetID and "images"):
//
//	GET  (empty)    — list all images
//	POST pull       — pull an image from JSON body {"image": "..."}
//	DELETE {id}     — remove an image
func (d *Deps) HandlePortainerImages(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *PortainerRuntime, subParts []string) {
	epID, err := portainerEndpointID(asset)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// GET /portainer/assets/{id}/images — list all images.
	if len(subParts) == 0 {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		images, err := runtime.Client.GetImages(ctx, epID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, images, nil)
		return
	}

	subAction := subParts[0]

	switch subAction {
	case "pull":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		var req struct {
			Image string `json:"image"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Image == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "image name required")
			return
		}
		if err := runtime.Client.PullImage(ctx, epID, req.Image); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, map[string]string{"status": "ok"}, nil)

	default:
		// Treat subAction as an image ID for DELETE.
		if r.Method != http.MethodDelete {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		imageID := subAction
		if imageID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "image id required")
			return
		}
		if err := runtime.Client.RemoveImage(ctx, epID, imageID); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, map[string]string{"status": "ok"}, nil)
	}
}

// handlePortainerVolumes dispatches volume-related sub-routes for a Portainer asset.
//
// Routes (subParts is parts[2:] after assetID and "volumes"):
//
//	GET    (empty)  — list all volumes
//	POST   (empty)  — create a volume from JSON body {"name": "...", "driver": "..."}
//	DELETE {name}   — remove a volume
func (d *Deps) HandlePortainerVolumes(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *PortainerRuntime, subParts []string) {
	epID, err := portainerEndpointID(asset)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(subParts) == 0 {
		switch r.Method {
		case http.MethodGet:
			volumes, err := runtime.Client.GetVolumes(ctx, epID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
				return
			}
			WritePortainerJSON(w, volumes, nil)

		case http.MethodPost:
			if !d.RequireAdminAuth(w, r) {
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "failed to read request body")
				return
			}
			var req struct {
				Name   string `json:"name"`
				Driver string `json:"driver"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			if req.Driver == "" {
				req.Driver = "local"
			}
			result, err := runtime.Client.CreateVolume(ctx, epID, req.Name, req.Driver)
			if err != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
				return
			}
			WritePortainerJSON(w, result, nil)

		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// subParts[0] is the volume name for DELETE.
	volumeName := subParts[0]
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.RequireAdminAuth(w, r) {
		return
	}
	if volumeName == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "volume name required")
		return
	}
	if err := runtime.Client.RemoveVolume(ctx, epID, volumeName); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
		return
	}
	WritePortainerJSON(w, map[string]string{"status": "ok"}, nil)
}

// handlePortainerNetworks dispatches network-related sub-routes for a Portainer asset.
//
// Routes (subParts is parts[2:] after assetID and "networks"):
//
//	GET    (empty)       — list all networks
//	POST   (empty)       — create a network from JSON body {"name": "...", "driver": "...", "subnet": "...", "gateway": "..."}
//	DELETE {networkID}   — remove a network
func (d *Deps) HandlePortainerNetworks(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *PortainerRuntime, subParts []string) {
	epID, err := portainerEndpointID(asset)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(subParts) == 0 {
		switch r.Method {
		case http.MethodGet:
			networks, err := runtime.Client.GetNetworks(ctx, epID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
				return
			}
			WritePortainerJSON(w, networks, nil)

		case http.MethodPost:
			if !d.RequireAdminAuth(w, r) {
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "failed to read request body")
				return
			}
			var req struct {
				Name    string `json:"name"`
				Driver  string `json:"driver"`
				Subnet  string `json:"subnet"`
				Gateway string `json:"gateway"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			if req.Driver == "" {
				req.Driver = "bridge"
			}
			result, err := runtime.Client.CreateNetwork(ctx, epID, req.Name, req.Driver, req.Subnet, req.Gateway)
			if err != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
				return
			}
			WritePortainerJSON(w, result, nil)

		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// subParts[0] is the network ID for DELETE.
	networkID := subParts[0]
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.RequireAdminAuth(w, r) {
		return
	}
	if networkID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "network id required")
		return
	}
	if err := runtime.Client.RemoveNetwork(ctx, epID, networkID); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
		return
	}
	WritePortainerJSON(w, map[string]string{"status": "ok"}, nil)
}
