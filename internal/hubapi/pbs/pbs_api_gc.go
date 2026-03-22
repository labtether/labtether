package pbs

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handlePBSGC handles GC status (GET) and GC trigger (POST) for a datastore.
// Routes:
//
//	GET  /pbs/assets/{assetID}/datastores/{ds}/gc  -> GC status from datastore status
//	POST /pbs/assets/{assetID}/datastores/{ds}/gc  -> trigger GC, returns UPID
func (d *Deps) HandlePBSDatastoreGC(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID, store string) {
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	store = strings.TrimSpace(store)
	if store == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "datastore name is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		status, statusErr := runtime.Client.GetDatastoreStatus(ctx, store, true)
		if statusErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch datastore status: "+statusErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"store":      store,
			"gc_status":  status.GCStatus,
			"fetched_at": time.Now().UTC().Format(time.RFC3339),
		})

	case http.MethodPost:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		upid, gcErr := runtime.Client.StartGC(ctx, store)
		if gcErr != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to start gc: "+gcErr.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
			"upid":  upid,
			"store": store,
		})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
