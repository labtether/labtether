package resources

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// resolvedAsset is the response envelope for an asset when resolve_composites=true.
// It mirrors assets.Asset but adds an optional Facets field for composite members.
type resolvedAsset struct {
	assets.Asset
	Facets []assetFacet `json:"facets,omitempty"`
}

// assetFacet describes one facet member of a resolved composite.
type assetFacet struct {
	AssetID string `json:"asset_id"`
	Source  string `json:"source"`
	Type    string `json:"type"`
}

// HandleAssets handles GET /assets.
func (d *Deps) HandleAssets(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/assets" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if groupID != "" {
		if d.GroupStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
			return
		}
		_, ok, err := d.GroupStore.GetGroup(groupID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load group")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "group not found")
			return
		}
	}

	var (
		assetList []assets.Asset
		err       error
	)
	if groupID != "" {
		if groupAssetStore, ok := d.AssetStore.(persistence.GroupAssetStore); ok {
			assetList, err = groupAssetStore.ListAssetsByGroup(groupID)
		} else {
			assetList, err = d.AssetStore.ListAssets()
			if err == nil {
				filtered := make([]assets.Asset, 0, len(assetList))
				for _, assetEntry := range assetList {
					if strings.TrimSpace(assetEntry.GroupID) == groupID {
						filtered = append(filtered, assetEntry)
					}
				}
				assetList = filtered
			}
		}
	} else {
		assetList, err = d.AssetStore.ListAssets()
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list assets")
		return
	}

	if tag := strings.TrimSpace(r.URL.Query().Get("tag")); tag != "" {
		normalized := assets.NormalizeTags([]string{tag})
		if len(normalized) == 0 {
			servicehttp.WriteError(w, http.StatusBadRequest, "tag cannot be empty")
			return
		}
		filtered := make([]assets.Asset, 0, len(assetList))
		for _, assetEntry := range assetList {
			if assetHasTag(assetEntry.Tags, normalized[0]) {
				filtered = append(filtered, assetEntry)
			}
		}
		assetList = filtered
	}

	resolveComposites := strings.TrimSpace(r.URL.Query().Get("resolve_composites")) == "true"
	if resolveComposites {
		resolved, err := d.resolveAssetComposites(assetList)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to resolve composites")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"assets": resolved})
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"assets": assetList})
}

// resolveAssetComposites fetches composite memberships for the given asset list,
// promotes primary assets with a facets annotation, and removes bare facet assets
// from the top-level response.
func (d *Deps) resolveAssetComposites(assetList []assets.Asset) ([]resolvedAsset, error) {
	// Build a quick lookup from asset ID to asset.
	assetByID := make(map[string]assets.Asset, len(assetList))
	assetIDs := make([]string, 0, len(assetList))
	for _, a := range assetList {
		assetByID[a.ID] = a
		assetIDs = append(assetIDs, a.ID)
	}

	// Fetch composites when the edge store is available.
	var composites []edges.Composite
	if d.EdgeStore != nil {
		var err error
		composites, err = d.EdgeStore.ListCompositesByAssets(assetIDs)
		if err != nil {
			return nil, err
		}
	}

	// For each composite, identify primary and facets; build a set of facet IDs
	// to suppress from the top-level list, and annotate primaries.
	facetAssetIDs := make(map[string]struct{})
	facetsByPrimary := make(map[string][]assetFacet)

	for _, comp := range composites {
		var primaryID string
		var facetMembers []edges.CompositeMember
		for _, m := range comp.Members {
			if m.Role == "primary" {
				primaryID = m.AssetID
			} else {
				facetMembers = append(facetMembers, m)
			}
		}
		if primaryID == "" {
			continue
		}
		for _, fm := range facetMembers {
			facetAssetIDs[fm.AssetID] = struct{}{}
			facetAsset, ok := assetByID[fm.AssetID]
			if !ok {
				continue
			}
			facetsByPrimary[primaryID] = append(facetsByPrimary[primaryID], assetFacet{
				AssetID: fm.AssetID,
				Source:  facetAsset.Source,
				Type:    facetAsset.Type,
			})
		}
	}

	// Build the resolved response: keep only non-facet assets and attach facets
	// to primaries.
	result := make([]resolvedAsset, 0, len(assetList))
	for _, a := range assetList {
		if _, isFacet := facetAssetIDs[a.ID]; isFacet {
			continue
		}
		ra := resolvedAsset{Asset: a}
		if facets, ok := facetsByPrimary[a.ID]; ok {
			ra.Facets = facets
		}
		result = append(result, ra)
	}
	return result, nil
}

func assetHasTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(tag), wanted) {
			return true
		}
	}
	return false
}

// HandleAssetActions handles all sub-paths under /assets/{id}.
func (d *Deps) HandleAssetActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/assets/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "asset path not found")
		return
	}

	if path == "heartbeat" {
		d.HandleRecordAssetHeartbeat(w, r)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		servicehttp.WriteError(w, http.StatusNotFound, "asset path not found")
		return
	}

	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "asset path not found")
		return
	}

	assetEntry, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load asset")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "asset not found")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"asset": assetEntry})
		case http.MethodPut, http.MethodPatch:
			if !d.EnforceRateLimit(w, r, "assets.update", 240, time.Minute) {
				return
			}

			var req assets.UpdateRequest
			if err := d.DecodeJSONBody(w, r, &req); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid asset update payload")
				return
			}
			if req.Name == nil && req.GroupID == nil && req.Tags == nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "at least one editable field is required")
				return
			}

			updateReq := assets.UpdateRequest{}
			if req.Name != nil {
				name := strings.TrimSpace(*req.Name)
				if name == "" {
					servicehttp.WriteError(w, http.StatusBadRequest, "name cannot be empty")
					return
				}
				if err := validateMaxLen("name", name, MaxPlanNameLength); err != nil {
					servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
					return
				}
				updateReq.Name = &name
			}
			if req.GroupID != nil {
				groupID := strings.TrimSpace(*req.GroupID)
				if groupID != "" {
					if d.GroupStore == nil {
						servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
						return
					}
					_, ok, err := d.GroupStore.GetGroup(groupID)
					if err != nil {
						servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate group")
						return
					}
					if !ok {
						servicehttp.WriteError(w, http.StatusBadRequest, "group_id does not reference an existing group")
						return
					}
				}
				updateReq.GroupID = &groupID
			}
			if req.Tags != nil {
				tags := assets.NormalizeTags(*req.Tags)
				if len(tags) > MaxAssetTagCount {
					servicehttp.WriteError(w, http.StatusBadRequest, "too many tags")
					return
				}
				for _, tag := range tags {
					if err := validateMaxLen("tag", tag, MaxAssetTagLength); err != nil {
						servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
						return
					}
				}
				updateReq.Tags = &tags
			}

			updatedAsset, err := d.AssetStore.UpdateAsset(assetEntry.ID, updateReq)
			if err != nil {
				if errors.Is(err, persistence.ErrNotFound) {
					servicehttp.WriteError(w, http.StatusNotFound, "asset not found")
					return
				}
				if errors.Is(err, groups.ErrGroupNotFound) {
					servicehttp.WriteError(w, http.StatusBadRequest, "group_id does not reference an existing group")
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update asset")
				return
			}
			if updateReq.GroupID != nil {
				if err := d.CascadeAssetSiteToInfraChildren(updatedAsset); err != nil {
					servicehttp.WriteError(w, http.StatusInternalServerError, "failed to cascade asset group assignment")
					return
				}
			}

			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"asset": updatedAsset})
		case http.MethodDelete:
			d.HandleDeleteAsset(w, assetEntry)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 3 && parts[1] == "terminal" && parts[2] == "config" {
		d.HandleAssetTerminalConfig(w, r, assetEntry)
		return
	}

	if len(parts) == 3 && parts[1] == "desktop" && parts[2] == "credentials" {
		if d.HandleDesktopCredentials != nil {
			d.HandleDesktopCredentials(w, r)
		} else {
			servicehttp.WriteError(w, http.StatusNotImplemented, "desktop credentials handler not configured")
		}
		return
	}

	if len(parts) == 4 && parts[1] == "desktop" && parts[2] == "credentials" && parts[3] == "retrieve" {
		if d.HandleRetrieveDesktopCredentials != nil {
			d.HandleRetrieveDesktopCredentials(w, r)
		} else {
			servicehttp.WriteError(w, http.StatusNotImplemented, "retrieve desktop credentials handler not configured")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "wake" {
		d.HandleWakeOnLAN(w, r, assetID)
		return
	}

	if len(parts) == 2 && parts[1] == "displays" {
		if d.HandleDisplayList != nil {
			d.HandleDisplayList(w, r, assetID)
		} else {
			servicehttp.WriteError(w, http.StatusNotImplemented, "display list handler not configured")
		}
		return
	}

	if len(parts) >= 2 && parts[1] == "dependencies" {
		d.HandleAssetDependencies(w, r, assetID, parts[2:])
		return
	}

	if len(parts) == 2 && parts[1] == "blast-radius" {
		d.HandleAssetBlastRadius(w, r, assetID)
		return
	}

	if len(parts) == 2 && parts[1] == "upstream" {
		d.HandleAssetUpstream(w, r, assetID)
		return
	}

	// POST /assets/{id}/protocols/ssh/push-hub-key
	// MUST come before the generic /test dispatch (both match len==4)
	if len(parts) == 4 && parts[1] == "protocols" && parts[2] == "ssh" && parts[3] == "push-hub-key" {
		if d.HandlePushHubKey != nil {
			d.HandlePushHubKey(w, r, assetID)
		} else {
			servicehttp.WriteError(w, http.StatusNotImplemented, "push hub key handler not configured")
		}
		return
	}

	// POST /assets/{id}/protocols/{protocol}/test
	if len(parts) == 4 && parts[1] == "protocols" && parts[3] == "test" {
		if d.HandleTestProtocolConnection != nil {
			d.HandleTestProtocolConnection(w, r, assetID, parts[2])
		} else {
			servicehttp.WriteError(w, http.StatusNotImplemented, "test protocol connection handler not configured")
		}
		return
	}

	// GET/POST /assets/{id}/protocols
	if len(parts) == 2 && parts[1] == "protocols" {
		switch r.Method {
		case http.MethodGet:
			if d.HandleListProtocolConfigs != nil {
				d.HandleListProtocolConfigs(w, r, assetID)
			} else {
				servicehttp.WriteError(w, http.StatusNotImplemented, "list protocol configs handler not configured")
			}
		case http.MethodPost:
			if d.HandleCreateProtocolConfig != nil {
				d.HandleCreateProtocolConfig(w, r, assetID)
			} else {
				servicehttp.WriteError(w, http.StatusNotImplemented, "create protocol config handler not configured")
			}
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// PUT/DELETE /assets/{id}/protocols/{protocol}
	if len(parts) == 3 && parts[1] == "protocols" {
		proto := parts[2]
		switch r.Method {
		case http.MethodPut:
			if d.HandleUpdateProtocolConfig != nil {
				d.HandleUpdateProtocolConfig(w, r, assetID, proto)
			} else {
				servicehttp.WriteError(w, http.StatusNotImplemented, "update protocol config handler not configured")
			}
		case http.MethodDelete:
			if d.HandleDeleteProtocolConfig != nil {
				d.HandleDeleteProtocolConfig(w, r, assetID, proto)
			} else {
				servicehttp.WriteError(w, http.StatusNotImplemented, "delete protocol config handler not configured")
			}
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "unknown asset action")
}
