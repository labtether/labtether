package pbs

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handlePBSVerifyJobs dispatches verify-job CRUD and run actions.
// Routes (relative to /pbs/assets/{assetID}/):
//
//	GET    verify-jobs           -> list
//	POST   verify-jobs           -> create
//	PUT    verify-jobs/{id}      -> update
//	DELETE verify-jobs/{id}      -> delete
//	POST   verify-jobs/{id}/run  -> run
func (d *Deps) HandlePBSVerifyJobs(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID string, subParts []string) {
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	// subParts[0] == "verify-jobs", subParts[1] (optional) == id or "", subParts[2] (optional) == "run"
	if len(subParts) == 1 {
		// Collection-level actions
		switch r.Method {
		case http.MethodGet:
			jobs, listErr := runtime.Client.ListVerifyJobs(ctx)
			if listErr != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to list verify jobs: "+listErr.Error())
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
				"jobs":       jobs,
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
			if createErr := runtime.Client.CreateVerifyJob(ctx, r.PostForm); createErr != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to create verify job: "+createErr.Error())
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "created"})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	jobID := strings.TrimSpace(subParts[1])
	if jobID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "verify job id is required")
		return
	}

	// Run sub-action
	if len(subParts) >= 3 && strings.TrimSpace(subParts[2]) == "run" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		upid, runErr := runtime.Client.RunVerifyJob(ctx, jobID)
		if runErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to run verify job: "+runErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"upid": upid})
		return
	}

	// Item-level actions
	switch r.Method {
	case http.MethodPut:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if parseErr := r.ParseForm(); parseErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid form body: "+parseErr.Error())
			return
		}
		if updateErr := runtime.Client.UpdateVerifyJob(ctx, jobID, r.PostForm); updateErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to update verify job: "+updateErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if deleteErr := runtime.Client.DeleteVerifyJob(ctx, jobID); deleteErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to delete verify job: "+deleteErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handlePBSPruneJobs dispatches prune-job CRUD and run actions.
// Routes (relative to /pbs/assets/{assetID}/):
//
//	GET    prune-jobs              -> list
//	POST   prune-jobs              -> create
//	PUT    prune-jobs/{id}         -> update
//	DELETE prune-jobs/{id}         -> delete
//	POST   prune-jobs/{id}/run     -> run
//	POST   prune-jobs/{id}/simulate -> simulate (dry-run)
func (d *Deps) HandlePBSPruneJobs(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID string, subParts []string) {
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	if len(subParts) == 1 {
		switch r.Method {
		case http.MethodGet:
			jobs, listErr := runtime.Client.ListPruneJobs(ctx)
			if listErr != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to list prune jobs: "+listErr.Error())
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
				"jobs":       jobs,
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
			if createErr := runtime.Client.CreatePruneJob(ctx, r.PostForm); createErr != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to create prune job: "+createErr.Error())
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "created"})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	jobID := strings.TrimSpace(subParts[1])
	if jobID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "prune job id is required")
		return
	}

	if len(subParts) >= 3 {
		subAction := strings.TrimSpace(subParts[2])
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		switch subAction {
		case "run":
			if !d.RequireAdminAuth(w, r) {
				return
			}
			upid, runErr := runtime.Client.RunPruneJob(ctx, jobID)
			if runErr != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to run prune job: "+runErr.Error())
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"upid": upid})
		default:
			servicehttp.WriteError(w, http.StatusNotFound, "unknown prune job action")
		}
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
		if updateErr := runtime.Client.UpdatePruneJob(ctx, jobID, r.PostForm); updateErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to update prune job: "+updateErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if deleteErr := runtime.Client.DeletePruneJob(ctx, jobID); deleteErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to delete prune job: "+deleteErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handlePBSSyncJobs dispatches sync-job CRUD and run actions.
// Routes (relative to /pbs/assets/{assetID}/):
//
//	GET    sync-jobs           -> list
//	POST   sync-jobs           -> create
//	PUT    sync-jobs/{id}      -> update
//	DELETE sync-jobs/{id}      -> delete
//	POST   sync-jobs/{id}/run  -> run
func (d *Deps) HandlePBSSyncJobs(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID string, subParts []string) {
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	if len(subParts) == 1 {
		switch r.Method {
		case http.MethodGet:
			jobs, listErr := runtime.Client.ListSyncJobs(ctx)
			if listErr != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to list sync jobs: "+listErr.Error())
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
				"jobs":       jobs,
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
			if createErr := runtime.Client.CreateSyncJob(ctx, r.PostForm); createErr != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "failed to create sync job: "+createErr.Error())
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "created"})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	jobID := strings.TrimSpace(subParts[1])
	if jobID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "sync job id is required")
		return
	}

	if len(subParts) >= 3 && strings.TrimSpace(subParts[2]) == "run" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		upid, runErr := runtime.Client.RunSyncJob(ctx, jobID)
		if runErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to run sync job: "+runErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"upid": upid})
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
		if updateErr := runtime.Client.UpdateSyncJob(ctx, jobID, r.PostForm); updateErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to update sync job: "+updateErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if deleteErr := runtime.Client.DeleteSyncJob(ctx, jobID); deleteErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to delete sync job: "+deleteErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
