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
		writePBSError(w, http.StatusBadGateway, "pbs runtime unavailable", err)
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
			writePBSError(w, http.StatusBadGateway, "failed to fetch datastore status", statusErr)
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
			writePBSError(w, http.StatusBadGateway, "failed to start gc", gcErr)
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

// HandlePBSDatastoreVerify starts an on-demand verification for a datastore.
// Route: POST /pbs/assets/{assetID}/datastores/{ds}/verify.
func (d *Deps) HandlePBSDatastoreVerify(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID, store string) {
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
		writePBSError(w, http.StatusBadGateway, "pbs runtime unavailable", err)
		return
	}

	upid, verifyErr := runtime.Client.StartVerify(ctx, store)
	if verifyErr != nil {
		writePBSError(w, http.StatusBadGateway, "failed to start verification", verifyErr)
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
		"upid":  upid,
		"store": store,
	})
}
