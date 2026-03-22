package resources

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/idgen"
)

// V2ListAssets handles GET /api/v2/assets.
func (d *Deps) V2ListAssets(w http.ResponseWriter, r *http.Request) {
	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list assets")
		return
	}

	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	var filtered []assets.Asset
	for _, a := range allAssets {
		if !apiv2.AssetCheck(allowed, a.ID) {
			continue
		}
		filtered = append(filtered, a)
	}

	// Apply query filters.
	if status := r.URL.Query().Get("status"); status != "" {
		var matching []assets.Asset
		for _, a := range filtered {
			if strings.EqualFold(a.Status, status) {
				matching = append(matching, a)
			}
		}
		filtered = matching
	}
	if platform := r.URL.Query().Get("platform"); platform != "" {
		var matching []assets.Asset
		for _, a := range filtered {
			if strings.EqualFold(a.Platform, platform) {
				matching = append(matching, a)
			}
		}
		filtered = matching
	}

	if filtered == nil {
		filtered = []assets.Asset{}
	}
	apiv2.WriteList(w, http.StatusOK, filtered, len(filtered), 1, len(filtered))
}

// V2GetAsset handles GET /api/v2/assets/{id}.
func (d *Deps) V2GetAsset(w http.ResponseWriter, _ *http.Request, assetID string) {
	asset, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to load asset")
		return
	}
	if !ok {
		apiv2.WriteError(w, http.StatusNotFound, "asset_not_found", "no asset with id: "+assetID)
		return
	}

	result := map[string]any{
		"asset": asset,
	}
	if d.AgentMgr != nil {
		result["agent_connected"] = d.AgentMgr.IsConnected(assetID)
	}
	apiv2.WriteJSON(w, http.StatusOK, result)
}

// V2CreateAsset handles POST /api/v2/assets.
func (d *Deps) V2CreateAsset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		IP       string `json:"ip"`
		Platform string `json:"platform"`
		GroupID  string `json:"group_id"`
	}
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.IP = strings.TrimSpace(req.IP)
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))

	if req.Name == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "name is required")
		return
	}

	assetID := idgen.New("asset")
	metadata := map[string]string{}
	if req.IP != "" {
		metadata["ip"] = req.IP
	}

	created, err := d.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  assetID,
		Type:     "host",
		Name:     req.Name,
		Source:   "manual",
		Status:   "unknown",
		Platform: req.Platform,
		GroupID:  strings.TrimSpace(req.GroupID),
		Metadata: metadata,
	})
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to create asset")
		return
	}
	apiv2.WriteJSON(w, http.StatusCreated, created)
}

// V2UpdateAsset handles PUT/PATCH /api/v2/assets/{id}.
func (d *Deps) V2UpdateAsset(w http.ResponseWriter, r *http.Request, assetID string) {
	var req assets.UpdateRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	if req.Name == nil && req.GroupID == nil && req.Tags == nil {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "at least one field required")
		return
	}

	updated, err := d.AssetStore.UpdateAsset(assetID, req)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to update asset")
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, updated)
}

// V2DeleteAsset handles DELETE /api/v2/assets/{id}.
func (d *Deps) V2DeleteAsset(w http.ResponseWriter, _ *http.Request, assetID string) {
	if err := d.AssetStore.DeleteAsset(assetID); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to delete asset")
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted", "asset_id": assetID})
}
