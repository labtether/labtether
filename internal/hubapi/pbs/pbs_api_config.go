package pbs

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handlePBSRemotes dispatches remote CRUD actions.
// Routes (relative to /pbs/assets/{assetID}/):
//   GET  remotes  -> list
//   POST remotes  -> create
func (d *Deps) HandlePBSRemotes(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID string) {
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		remotes, listErr := runtime.Client.ListRemotes(ctx)
		if listErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to list remotes: "+listErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"remotes":    remotes,
			"fetched_at": time.Now().UTC().Format(time.RFC3339),
		})
	case http.MethodPost:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if parseErr := r.ParseForm(); parseErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid form body: "+parseErr.Error())
			return
		}
		if createErr := runtime.Client.CreateRemote(ctx, r.PostForm); createErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to create remote: "+createErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "created"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handlePBSTrafficControl dispatches traffic-control CRUD actions.
// Routes (relative to /pbs/assets/{assetID}/):
//   GET    traffic-control         -> list
//   POST   traffic-control         -> create
//   PUT    traffic-control/{name}  -> update
//   DELETE traffic-control/{name}  -> delete
func (d *Deps) HandlePBSTrafficControl(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID string, subParts []string) {
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	if len(subParts) == 1 {
		switch r.Method {
		case http.MethodGet:
			rules, listErr := runtime.Client.ListTrafficControl(ctx)
			if listErr != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to list traffic control rules: "+listErr.Error())
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
				"rules":      rules,
				"fetched_at": time.Now().UTC().Format(time.RFC3339),
			})
		case http.MethodPost:
			if !d.RequireAdminAuth(w, r) {
				return
			}
			if parseErr := r.ParseForm(); parseErr != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid form body: "+parseErr.Error())
				return
			}
			if createErr := runtime.Client.CreateTrafficControl(ctx, r.PostForm); createErr != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to create traffic control rule: "+createErr.Error())
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "created"})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	ruleName := strings.TrimSpace(subParts[1])
	if ruleName == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "traffic control rule name is required")
		return
	}

	switch r.Method {
	case http.MethodPut:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if parseErr := r.ParseForm(); parseErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid form body: "+parseErr.Error())
			return
		}
		if updateErr := runtime.Client.UpdateTrafficControl(ctx, ruleName, r.PostForm); updateErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to update traffic control rule: "+updateErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if deleteErr := runtime.Client.DeleteTrafficControl(ctx, ruleName); deleteErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to delete traffic control rule: "+deleteErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handlePBSCertificates handles GET /pbs/assets/{assetID}/certificates.
func (d *Deps) HandlePBSCertificates(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	certs, certsErr := runtime.Client.GetCertificateInfo(ctx)
	if certsErr != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch certificate info: "+certsErr.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"certificates": certs,
		"fetched_at":   time.Now().UTC().Format(time.RFC3339),
	})
}

// handlePBSDatastoreMaintenance handles POST /pbs/assets/{assetID}/datastores/{ds}/maintenance.
// Expects form field "mode" (e.g. "read-only", "offline", or "" to clear).
func (d *Deps) HandlePBSDatastoreMaintenance(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID, store string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.RequireAdminAuth(w, r) {
		return
	}

	store = strings.TrimSpace(store)
	if store == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "datastore name is required")
		return
	}

	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	if parseErr := r.ParseForm(); parseErr != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid form body: "+parseErr.Error())
		return
	}
	mode := strings.TrimSpace(r.FormValue("mode"))

	if maintenanceErr := runtime.Client.SetDatastoreMaintenanceMode(ctx, store, mode); maintenanceErr != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to set maintenance mode: "+maintenanceErr.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
		"store": store,
		"mode":  mode,
	})
}
