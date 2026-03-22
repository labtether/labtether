package collectors

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleHubCollectors(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/hub-collectors" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.HubCollectorStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub collector store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		enabledOnly := strings.ToLower(r.URL.Query().Get("enabled")) == "true"
		collectors, err := d.HubCollectorStore.ListHubCollectors(shared.ParseLimit(r, 50), enabledOnly)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list hub collectors")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"collectors": collectors})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "hubcollector.create", 120, time.Minute) {
			return
		}
		var req hubcollector.CreateCollectorRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid hub collector payload")
			return
		}
		req.AssetID = strings.TrimSpace(req.AssetID)
		if req.AssetID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "asset_id is required")
			return
		}
		if normalized := hubcollector.NormalizeCollectorType(req.CollectorType); normalized == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "collector_type must be one of: ssh, winrm, api, proxmox, pbs, truenas, portainer, docker, homeassistant")
			return
		} else {
			req.CollectorType = normalized
		}
		collector, err := d.HubCollectorStore.CreateHubCollector(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create hub collector")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"collector": collector})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleHubCollectorActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/hub-collectors/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "hub collector path not found")
		return
	}
	if d.HubCollectorStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub collector store unavailable")
		return
	}

	parts := strings.Split(path, "/")
	collectorID := strings.TrimSpace(parts[0])
	if collectorID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "hub collector path not found")
		return
	}

	if len(parts) == 2 && parts[1] == "run" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.EnforceRateLimit(w, r, "hubcollector.run", 240, time.Minute) {
			return
		}

		if err := d.RunHubCollectorNow(collectorID); err != nil {
			if err == hubcollector.ErrCollectorNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "hub collector not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to start collector run")
			return
		}

		servicehttp.WriteJSON(w, http.StatusAccepted, map[string]any{
			"status":       "started",
			"collector_id": collectorID,
			"message":      "collector run started",
		})
		return
	}

	if len(parts) > 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown hub collector action")
		return
	}

	// GET/PATCH/DELETE /hub-collectors/{id}
	switch r.Method {
	case http.MethodGet:
		collector, ok, err := d.HubCollectorStore.GetHubCollector(collectorID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load hub collector")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "hub collector not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"collector": collector})
	case http.MethodPatch, http.MethodPut:
		if !d.EnforceRateLimit(w, r, "hubcollector.update", 180, time.Minute) {
			return
		}
		var req hubcollector.UpdateCollectorRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid hub collector payload")
			return
		}
		updated, err := d.HubCollectorStore.UpdateHubCollector(collectorID, req)
		if err != nil {
			if err == hubcollector.ErrCollectorNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "hub collector not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update hub collector")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"collector": updated})
	case http.MethodDelete:
		if err := d.HubCollectorStore.DeleteHubCollector(collectorID); err != nil {
			if err == hubcollector.ErrCollectorNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "hub collector not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete hub collector")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) RunHubCollectorNow(collectorID string) error {
	collector, ok, err := d.HubCollectorStore.GetHubCollector(collectorID)
	if err != nil {
		return err
	}
	if !ok {
		return hubcollector.ErrCollectorNotFound
	}

	d.UpdateCollectorStatus(collector.ID, "running", "")
	d.startCollectorRun(context.Background(), collector, true)

	return nil
}
