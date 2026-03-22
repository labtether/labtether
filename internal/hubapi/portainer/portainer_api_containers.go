package portainer

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"net/http"
	"strconv"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

// handlePortainerContainers dispatches container-related sub-routes for a Portainer asset.
//
// Routes (subParts is parts[2:] after assetID and "containers"):
//
//	GET  (empty)          — list all containers
//	GET  {cid}/logs       — container logs
//	GET  {cid}/inspect    — container inspect
//	POST {cid}/start      — start container
//	POST {cid}/stop       — stop container
//	POST {cid}/restart    — restart container
//	POST {cid}/kill       — kill container
//	POST {cid}/remove     — remove container
func (d *Deps) HandlePortainerContainers(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *PortainerRuntime, subParts []string) {
	epID, err := portainerEndpointID(asset)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// GET /portainer/assets/{id}/containers — list all containers.
	if len(subParts) == 0 {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		containers, err := runtime.Client.GetContainers(ctx, epID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, containers, nil)
		return
	}

	// subParts[0] is the container ID; subParts[1] (if present) is the sub-action.
	containerID := subParts[0]
	if containerID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "container id required")
		return
	}

	if len(subParts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown container action")
		return
	}

	subAction := subParts[1]

	switch subAction {
	case "logs":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		tail := 500
		if t := r.URL.Query().Get("tail"); t != "" {
			if v, err := strconv.Atoi(t); err == nil && v > 0 {
				tail = v
			}
		}
		timestamps := r.URL.Query().Get("timestamps") == "true"
		logs, err := runtime.Client.GetContainerLogs(ctx, epID, containerID, tail, timestamps)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, logs, nil)

	case "inspect":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		data, err := runtime.Client.InspectContainer(ctx, epID, containerID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, data, nil)

	case "start", "stop", "restart", "kill":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if err := runtime.Client.ContainerAction(ctx, epID, containerID, subAction); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, map[string]string{"status": "ok"}, nil)

	case "remove":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		force := r.URL.Query().Get("force") == "true"
		if err := runtime.Client.RemoveContainer(ctx, epID, containerID, force); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, map[string]string{"status": "ok"}, nil)

	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown container action")
	}
}
