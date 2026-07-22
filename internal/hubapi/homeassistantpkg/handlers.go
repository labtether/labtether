package homeassistantpkg

import (
	"io"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

type entityActionRequest struct {
	AssetID     string            `json:"asset_id,omitempty"`
	CollectorID string            `json:"collector_id,omitempty"`
	Action      string            `json:"action"`
	Service     string            `json:"service,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	DryRun      bool              `json:"dry_run,omitempty"`
}

func (d *Deps) HandleV2HAEntities(w http.ResponseWriter, r *http.Request) {
	scope := "homeassistant:read"
	if r.Method == http.MethodPost {
		scope = "homeassistant:write"
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	if apiv2.IsMutatingMethod(r.Method) && !d.requireMutationAdmin(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.writeEntityList(w, r, "")
	case http.MethodPost:
		var req entityActionRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil && err != io.EOF {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid Home Assistant entity action payload")
			return
		}
		if strings.TrimSpace(req.AssetID) == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "asset_id is required")
			return
		}
		d.executeEntityAction(w, r, req.AssetID, req)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleV2HAEntityActions(w http.ResponseWriter, r *http.Request) {
	scope := "homeassistant:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "homeassistant:write"
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	if apiv2.IsMutatingMethod(r.Method) && !d.requireMutationAdmin(w, r) {
		return
	}
	assetID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v2/homeassistant/entities/"))
	if assetID == "" || strings.Contains(assetID, "/") {
		servicehttp.WriteError(w, http.StatusNotFound, "Home Assistant entity not found")
		return
	}
	if !apiv2.RequireAssetAccess(w, r, assetID) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		asset, ok := d.getEntity(w, assetID)
		if !ok {
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"entity": asset})
	case http.MethodPost:
		var req entityActionRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil && err != io.EOF {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid Home Assistant entity action payload")
			return
		}
		d.executeEntityAction(w, r, assetID, req)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) requireMutationAdmin(w http.ResponseWriter, r *http.Request) bool {
	if d.RequireAdminAuth == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "admin authorization unavailable")
		return false
	}
	return d.RequireAdminAuth(w, r)
}

func (d *Deps) HandleV2HAAutomations(w http.ResponseWriter, r *http.Request) {
	d.handleEntityCollection(w, r, "automation")
}

func (d *Deps) HandleV2HAScenes(w http.ResponseWriter, r *http.Request) {
	d.handleEntityCollection(w, r, "scene")
}

func (d *Deps) handleEntityCollection(w http.ResponseWriter, r *http.Request, domain string) {
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "homeassistant:read") {
		apiv2.WriteScopeForbidden(w, "homeassistant:read")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	d.writeEntityList(w, r, domain)
}

func (d *Deps) writeEntityList(w http.ResponseWriter, r *http.Request, requiredDomain string) {
	if d.AssetStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset store unavailable")
		return
	}
	all, err := d.AssetStore.ListAssets()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list Home Assistant entities")
		return
	}
	all = shared.FilterAssetsByAccess(r.Context(), all)
	collectorFilter := strings.TrimSpace(r.URL.Query().Get("collector_id"))
	domainFilter := strings.TrimSpace(r.URL.Query().Get("domain"))
	if requiredDomain != "" {
		domainFilter = requiredDomain
	}
	stateFilter := strings.TrimSpace(r.URL.Query().Get("state"))
	entities := make([]assets.Asset, 0)
	for _, asset := range all {
		if !isHomeAssistantEntity(asset) {
			continue
		}
		if collectorFilter != "" && strings.TrimSpace(asset.Metadata["collector_id"]) != collectorFilter {
			continue
		}
		if domainFilter != "" && !strings.EqualFold(strings.TrimSpace(asset.Metadata["domain"]), domainFilter) {
			continue
		}
		if stateFilter != "" && !strings.EqualFold(strings.TrimSpace(asset.Metadata["state"]), stateFilter) {
			continue
		}
		entities = append(entities, asset)
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"entities": entities, "count": len(entities)})
}

func (d *Deps) getEntity(w http.ResponseWriter, assetID string) (assets.Asset, bool) {
	if d.AssetStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset store unavailable")
		return assets.Asset{}, false
	}
	asset, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load Home Assistant entity")
		return assets.Asset{}, false
	}
	if !ok || !isHomeAssistantEntity(asset) {
		servicehttp.WriteError(w, http.StatusNotFound, "Home Assistant entity not found")
		return assets.Asset{}, false
	}
	return asset, true
}

func (d *Deps) executeEntityAction(w http.ResponseWriter, r *http.Request, assetID string, req entityActionRequest) {
	if !apiv2.RequireAssetAccess(w, r, assetID) {
		return
	}
	asset, ok := d.getEntity(w, assetID)
	if !ok {
		return
	}
	if len(req.Params) > 32 {
		servicehttp.WriteError(w, http.StatusBadRequest, "too many action parameters")
		return
	}
	if len(req.Action) > 128 || len(req.Service) > 256 || len(req.CollectorID) > 256 {
		servicehttp.WriteError(w, http.StatusBadRequest, "Home Assistant action fields exceed size limits")
		return
	}
	for key, value := range req.Params {
		if strings.TrimSpace(key) == "" || len(key) > 128 || len(value) > 4096 {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid action parameter")
			return
		}
	}

	runtime, err := d.loadRuntimeForAsset(asset, req.CollectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusConflict, "Home Assistant collector is not available for the selected entity")
		return
	}
	entityID := strings.TrimSpace(asset.Metadata["entity_id"])
	if entityID == "" {
		servicehttp.WriteError(w, http.StatusConflict, "Home Assistant entity is missing entity_id metadata")
		return
	}
	actionID, params, err := resolveEntityAction(asset, req)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := runtime.Connector.ExecuteAction(r.Context(), actionID, connectorsdk.ActionRequest{
		TargetID: entityID,
		Params:   params,
		DryRun:   req.DryRun,
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "Home Assistant action execution failed")
		return
	}
	status := http.StatusOK
	if strings.EqualFold(result.Status, "failed") {
		status = http.StatusBadRequest
		result.Message = shared.SanitizeUpstreamError(result.Message)
	}
	servicehttp.WriteJSON(w, status, map[string]any{
		"asset_id":     asset.ID,
		"entity_id":    entityID,
		"collector_id": runtime.CollectorID,
		"result":       result,
	})
}

func resolveEntityAction(asset assets.Asset, req entityActionRequest) (string, map[string]string, error) {
	action := strings.ToLower(strings.TrimSpace(req.Action))
	service := strings.ToLower(strings.TrimSpace(req.Service))
	params := make(map[string]string, len(req.Params)+1)
	for key, value := range req.Params {
		params[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if action == "" && service != "" {
		action = "service.call"
	}
	switch action {
	case "toggle", "entity.toggle":
		return "entity.toggle", params, nil
	case "turn_on", "turn_off", "open_cover", "close_cover", "lock", "unlock", "press", "trigger":
		domain := strings.ToLower(strings.TrimSpace(asset.Metadata["domain"]))
		if domain == "" {
			return "", nil, serviceError("entity domain metadata is missing")
		}
		service = domain + "." + action
		action = "service.call"
	case "service.call":
		if service == "" {
			service = strings.ToLower(strings.TrimSpace(params["service"]))
		}
	default:
		if strings.Count(action, ".") == 1 {
			service = action
			action = "service.call"
		} else {
			return "", nil, serviceError("unsupported Home Assistant entity action")
		}
	}
	if action == "service.call" {
		parts := strings.Split(service, ".")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", nil, serviceError("service must use domain.action format")
		}
		entityDomain := strings.ToLower(strings.TrimSpace(asset.Metadata["domain"]))
		if parts[0] != entityDomain && parts[0] != "homeassistant" {
			return "", nil, serviceError("service domain must match the selected entity")
		}
		if parts[0] == "homeassistant" && !safeUniversalEntityService(parts[1]) {
			return "", nil, serviceError("homeassistant service is not an entity-scoped action")
		}
		params["service"] = service
	}
	return action, params, nil
}

func safeUniversalEntityService(action string) bool {
	switch action {
	case "toggle", "turn_on", "turn_off", "update_entity":
		return true
	default:
		return false
	}
}

type serviceError string

func (e serviceError) Error() string { return string(e) }

func isHomeAssistantEntity(asset assets.Asset) bool {
	source := strings.ToLower(strings.TrimSpace(asset.Source))
	if source != "homeassistant" && source != "home-assistant" {
		return false
	}
	return strings.TrimSpace(asset.Metadata["entity_id"]) != ""
}
