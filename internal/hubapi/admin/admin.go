package admin

import (
	"log"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// HandleAdminReset handles POST /admin/reset to wipe all operational data.
// Requires a JSON body with {"confirm": "RESET"} for safety.
func (d *Deps) HandleAdminReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := d.decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Confirm != "RESET" {
		servicehttp.WriteError(w, http.StatusBadRequest, "confirmation required: send {\"confirm\": \"RESET\"}")
		return
	}

	actorID := d.principalActorID(r.Context())
	log.Printf("audit: admin reset triggered by actor=%s at=%s remote=%s", actorID, time.Now().UTC().Format(time.RFC3339), r.RemoteAddr) // #nosec G706 -- Actor and remote address are trusted request metadata, not free-form log text.

	result, err := d.AdminResetStore.ResetAllData()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "reset failed: "+err.Error())
		return
	}

	// Flush in-memory caches so the UI reflects the clean state immediately.
	if d.WebServiceCoordinator != nil {
		d.WebServiceCoordinator.ClearAll()
	}
	if d.DockerCoordinator != nil {
		d.DockerCoordinator.ClearAll()
	}
	if d.InvalidateStatusCaches != nil {
		d.InvalidateStatusCaches()
	}

	servicehttp.WriteJSON(w, http.StatusOK, result)
}
