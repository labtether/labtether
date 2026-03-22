package portainer

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

// handlePortainerStacks dispatches stack-related sub-routes for a Portainer asset.
//
// Routes (subParts is parts[2:] after assetID and "stacks"):
//
//	GET  (empty)          — list all stacks
//	GET  {sid}/compose    — get compose file content
//	PUT  {sid}/compose    — update compose file content
//	POST {sid}/start      — start a stopped stack
//	POST {sid}/stop       — stop a running stack
//	POST {sid}/redeploy   — redeploy a git-based stack
//	POST {sid}/remove     — remove a stack
func (d *Deps) HandlePortainerStacks(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *PortainerRuntime, subParts []string) {
	epID, err := portainerEndpointID(asset)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// GET /portainer/assets/{id}/stacks — list all stacks.
	if len(subParts) == 0 {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		stacks, err := runtime.Client.GetStacks(ctx)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, stacks, nil)
		return
	}

	// subParts[0] is the stack ID; subParts[1] (if present) is the sub-action.
	stackIDStr := subParts[0]
	if stackIDStr == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "stack id required")
		return
	}
	stackID, err := strconv.Atoi(stackIDStr)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid stack id")
		return
	}

	if len(subParts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown stack action")
		return
	}

	subAction := subParts[1]

	switch subAction {
	case "compose":
		switch r.Method {
		case http.MethodGet:
			content, err := runtime.Client.GetStackCompose(ctx, stackID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
				return
			}
			WritePortainerJSON(w, content, nil)

		case http.MethodPut:
			if !d.RequireAdminAuth(w, r) {
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "failed to read request body")
				return
			}
			var req struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			if err := runtime.Client.UpdateStackCompose(ctx, stackID, epID, req.Content); err != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
				return
			}
			WritePortainerJSON(w, map[string]string{"status": "ok"}, nil)

		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	case "start":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := runtime.Client.StartStack(ctx, stackID, epID); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, map[string]string{"status": "ok"}, nil)

	case "stop":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := runtime.Client.StopStack(ctx, stackID, epID); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, map[string]string{"status": "ok"}, nil)

	case "redeploy":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		pullImage := r.URL.Query().Get("pull") == "true"
		if err := runtime.Client.RedeployStack(ctx, stackID, epID, pullImage); err != nil {
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
		if err := runtime.Client.RemoveStack(ctx, stackID, epID); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
			return
		}
		WritePortainerJSON(w, map[string]string{"status": "ok"}, nil)

	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown stack action")
	}
}
