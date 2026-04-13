package resources

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/servicehttp"
)

// validManualDevicePlatforms is the set of accepted platform values for manual devices.
// Empty string is permitted (platform unknown).
var validManualDevicePlatforms = map[string]struct{}{
	"":        {},
	"linux":   {},
	"windows": {},
	"macos":   {},
	"freebsd": {},
	"other":   {},
}

type manualDeviceCreateRequest struct {
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Platform string   `json:"platform,omitempty"`
	GroupID  string   `json:"group_id,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// HandleManualDeviceRoutes handles POST /assets/manual.
func (d *Deps) HandleManualDeviceRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req manualDeviceCreateRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.GroupID = strings.TrimSpace(req.GroupID)

	if req.Name == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Host == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "host is required")
		return
	}
	if err := protocols.ValidateManualDeviceHost(req.Host); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok := validManualDevicePlatforms[req.Platform]; !ok {
		servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid platform %q: must be one of linux, windows, macos, freebsd, other, or empty", req.Platform))
		return
	}

	if req.GroupID != "" && d.GroupStore != nil {
		_, ok, err := d.GroupStore.GetGroup(req.GroupID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate group")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusBadRequest, "group_id does not reference an existing group")
			return
		}
	}

	assetID := idgen.New("manual")

	heartbeatReq := assets.HeartbeatRequest{
		AssetID:  assetID,
		Type:     "device",
		Name:     req.Name,
		Source:   "manual",
		GroupID:  req.GroupID,
		Status:   "online",
		Platform: req.Platform,
	}

	assetEntry, err := d.AssetStore.UpsertAssetHeartbeat(heartbeatReq)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create asset")
		return
	}

	// Set host and transport_type — these are not part of HeartbeatRequest.
	if d.ManualDeviceDB != nil {
		if _, execErr := d.ManualDeviceDB.Exec(context.Background(),
			`UPDATE assets SET host = $1, transport_type = 'manual' WHERE id = $2`,
			req.Host,
			assetID,
		); execErr != nil {
			_ = d.AssetStore.DeleteAsset(assetID)
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to set host on asset")
			return
		}
	}
	assetEntry.Host = req.Host
	assetEntry.TransportType = "manual"

	// Apply tags via UpdateAsset if provided.
	if len(req.Tags) > 0 {
		normalized := assets.NormalizeTags(req.Tags)
		updateReq := assets.UpdateRequest{Tags: &normalized}
		updatedAsset, updateErr := d.AssetStore.UpdateAsset(assetID, updateReq)
		if updateErr != nil {
			_ = d.AssetStore.DeleteAsset(assetID)
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to set tags on asset")
			return
		}
		assetEntry = updatedAsset
		assetEntry.Host = req.Host
		assetEntry.TransportType = "manual"
	}

	actorID := ""
	if d.PrincipalActorID != nil {
		actorID = d.PrincipalActorID(r.Context())
	}
	ev := audit.NewEvent("asset.manual.created")
	ev.ActorID = actorID
	ev.Target = assetID
	ev.Details = map[string]any{
		"name": req.Name,
		"host": req.Host,
	}
	if d.AppendAuditEventBestEffort != nil {
		d.AppendAuditEventBestEffort(ev, "api warning: failed to append manual device create audit event")
	}

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"asset": assetEntry})
}
