package pbs

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handlePBSSnapshotVerify triggers an ad-hoc verify for a datastore.
// POST /pbs/assets/{assetID}/snapshots/verify?store={store}
func (d *Deps) HandlePBSSnapshotVerify(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.RequireAdminAuth(w, r) {
		return
	}

	store := strings.TrimSpace(r.URL.Query().Get("store"))
	if store == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "store query parameter is required")
		return
	}

	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	upid, verifyErr := runtime.Client.StartVerify(ctx, store)
	if verifyErr != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to start verify: "+verifyErr.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
		"upid":  upid,
		"store": store,
	})
}

// handlePBSSnapshotForget deletes a single snapshot.
// DELETE /pbs/assets/{assetID}/snapshots/forget?store={store}&backup-type={type}&backup-id={id}&backup-time={time}
func (d *Deps) HandlePBSSnapshotForget(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID string) {
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.RequireAdminAuth(w, r) {
		return
	}

	store := strings.TrimSpace(r.URL.Query().Get("store"))
	backupType := strings.TrimSpace(r.URL.Query().Get("backup-type"))
	backupID := strings.TrimSpace(r.URL.Query().Get("backup-id"))
	backupTimeStr := strings.TrimSpace(r.URL.Query().Get("backup-time"))

	if store == "" || backupType == "" || backupID == "" || backupTimeStr == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "store, backup-type, backup-id, and backup-time are required")
		return
	}

	backupTime, parseErr := strconv.ParseInt(backupTimeStr, 10, 64)
	if parseErr != nil || backupTime <= 0 {
		servicehttp.WriteError(w, http.StatusBadRequest, "backup-time must be a positive unix timestamp")
		return
	}

	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	if forgetErr := runtime.Client.ForgetSnapshot(ctx, store, backupType, backupID, backupTime); forgetErr != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to forget snapshot: "+forgetErr.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handlePBSGroupForget deletes all snapshots in a backup group.
// DELETE /pbs/assets/{assetID}/groups/forget?store={store}&backup-type={type}&backup-id={id}
func (d *Deps) HandlePBSGroupForget(ctx context.Context, w http.ResponseWriter, r *http.Request, collectorID string) {
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.RequireAdminAuth(w, r) {
		return
	}

	store := strings.TrimSpace(r.URL.Query().Get("store"))
	backupType := strings.TrimSpace(r.URL.Query().Get("backup-type"))
	backupID := strings.TrimSpace(r.URL.Query().Get("backup-id"))

	if store == "" || backupType == "" || backupID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "store, backup-type, and backup-id are required")
		return
	}

	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs runtime unavailable: "+err.Error())
		return
	}

	if forgetErr := runtime.Client.ForgetGroup(ctx, store, backupType, backupID); forgetErr != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to forget group: "+forgetErr.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
