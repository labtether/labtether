package alerting

import (
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strings"
	"time"
)

func (d *Deps) HandleIncidentActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/incidents/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "incident path not found")
		return
	}
	if d.IncidentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "incident store unavailable")
		return
	}

	parts := strings.Split(path, "/")
	incidentID := strings.TrimSpace(parts[0])
	if incidentID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "incident path not found")
		return
	}
	existingIncident, ok, err := d.IncidentStore.GetIncident(incidentID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load incident")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}
	if !d.requireIncidentAccess(w, r, existingIncident) {
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			incident, ok, err := d.IncidentStore.GetIncident(incidentID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load incident")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"incident": incident})
		case http.MethodPatch, http.MethodPut:
			if !d.EnforceRateLimit(w, r, "incidents.update", 180, time.Minute) {
				return
			}

			var req incidents.UpdateIncidentRequest
			if err := decodeJSONBody(w, r, &req); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid incident payload")
				return
			}
			normalizeUpdateIncidentRequest(&req)
			if err := validateUpdateIncidentRequest(req); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}

			groupID := ""
			if req.GroupID != nil {
				groupID = strings.TrimSpace(*req.GroupID)
			}
			assetID := ""
			if req.PrimaryAssetID != nil {
				assetID = strings.TrimSpace(*req.PrimaryAssetID)
			}
			if req.GroupID != nil || req.PrimaryAssetID != nil {
				if err := d.validateIncidentReferences(groupID, assetID); err != nil {
					if errors.Is(err, errIncidentGroupStoreUnavailable) || errors.Is(err, errIncidentAssetStoreUnavailable) {
						servicehttp.WriteError(w, http.StatusServiceUnavailable, err.Error())
						return
					}
					if errors.Is(err, groups.ErrGroupNotFound) {
						servicehttp.WriteError(w, http.StatusNotFound, "group not found")
						return
					}
					if strings.Contains(strings.ToLower(err.Error()), "asset not found") {
						servicehttp.WriteError(w, http.StatusNotFound, err.Error())
						return
					}
					servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
					return
				}
			}
			if shared.HasAssetRestriction(r.Context()) {
				prospective := existingIncident
				if req.GroupID != nil {
					prospective.GroupID = strings.TrimSpace(*req.GroupID)
				}
				if req.PrimaryAssetID != nil {
					prospective.PrimaryAssetID = strings.TrimSpace(*req.PrimaryAssetID)
				}
				groupAccess, authErr := d.accessibleGroupIDs(r.Context())
				allowed, checkErr := d.incidentAllowed(r.Context(), prospective, groupAccess)
				if authErr != nil || checkErr != nil || !allowed {
					writeAssetScopeForbidden(w, "incident update would exceed the api key asset allowlist")
					return
				}
			}

			updated, err := d.IncidentStore.UpdateIncident(incidentID, req)
			if err != nil {
				if errors.Is(err, incidents.ErrIncidentNotFound) {
					servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
					return
				}
				if errors.Is(err, incidents.ErrInvalidStatusTransition) ||
					strings.Contains(strings.ToLower(err.Error()), "invalid") {
					servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update incident")
				return
			}
			if event := incidentMaterialTransitionEvent(existingIncident, updated); event != "" {
				d.dispatchIncidentNotificationAsync(updated, event)
			} else if incidentLiveActivityContentChanged(existingIncident, updated) {
				// Content-only changes refresh an existing Live Activity without
				// producing another user-visible incident notification.
				d.dispatchIncidentNotificationAsync(updated, "incident.updated")
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"incident": updated})
		case http.MethodDelete:
			if d.DependencyStore != nil {
				linkedAssets, err := d.DependencyStore.ListIncidentAssets(incidentID, 500)
				if err != nil && !errors.Is(err, incidents.ErrIncidentNotFound) {
					servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load incident assets")
					return
				}
				for _, linkedAsset := range linkedAssets {
					if err := d.DependencyStore.UnlinkIncidentAsset(incidentID, linkedAsset.ID); err != nil &&
						!errors.Is(err, persistence.ErrNotFound) &&
						!errors.Is(err, incidents.ErrIncidentNotFound) {
						servicehttp.WriteError(w, http.StatusInternalServerError, "failed to unlink incident asset")
						return
					}
				}
			}

			if err := d.IncidentStore.DeleteIncident(incidentID); err != nil {
				if errors.Is(err, incidents.ErrIncidentNotFound) {
					servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete incident")
				return
			}
			finalIncident := existingIncident
			finalIncident.Status = incidents.StatusClosed
			finalIncident.UpdatedAt = time.Now().UTC()
			d.dispatchIncidentNotificationAsync(finalIncident, "incident.resolved")
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "incident_id": incidentID})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "link-alert" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.EnforceRateLimit(w, r, "incidents.link_alert", 240, time.Minute) {
			return
		}

		var req incidents.LinkAlertRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid incident link payload")
			return
		}
		normalizeLinkAlertRequest(&req)
		// Incident-link attribution is authoritative server context, not a
		// caller-selected label.
		req.CreatedBy = apiv2.PrincipalActorID(r.Context())
		if err := validateLinkAlertRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.AlertRuleID != "" {
			if d.AlertStore == nil {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "alert store unavailable")
				return
			}
			rule, ok, err := d.AlertStore.GetAlertRule(req.AlertRuleID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert rule")
				return
			} else if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
				return
			}
			if !d.requireAlertRuleAccess(w, r, rule) {
				return
			}
		}
		if shared.HasAssetRestriction(r.Context()) && req.AlertInstanceID != "" {
			if d.AlertInstanceStore == nil {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "alert instance store unavailable")
				return
			}
			instance, found, loadErr := d.AlertInstanceStore.GetAlertInstance(req.AlertInstanceID)
			if loadErr != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert instance")
				return
			}
			if !found {
				servicehttp.WriteError(w, http.StatusNotFound, "alert instance not found")
				return
			}
			groupAccess, authErr := d.accessibleGroupIDs(r.Context())
			allowed, checkErr := d.alertInstanceAllowed(r.Context(), instance, groupAccess)
			if authErr != nil || checkErr != nil || !allowed {
				writeAssetScopeForbidden(w, "api key does not have access to this alert instance")
				return
			}
		}
		if shared.HasAssetRestriction(r.Context()) && req.AlertRuleID == "" && req.AlertInstanceID == "" {
			writeAssetScopeForbidden(w, "fingerprint-only alert links cannot be proven to stay within the asset allowlist")
			return
		}

		link, err := d.IncidentStore.LinkIncidentAlert(incidentID, req)
		if err != nil {
			if errors.Is(err, incidents.ErrIncidentNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
				return
			}
			if errors.Is(err, incidents.ErrAlertReferenceRequired) {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			if errors.Is(err, incidents.ErrIncidentAlertLinkConflict) {
				servicehttp.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			if strings.Contains(strings.ToLower(err.Error()), "invalid") {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to link incident alert")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"link": link})
		return
	}

	if len(parts) == 2 && parts[1] == "alerts" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		links, err := d.IncidentStore.ListIncidentAlertLinks(incidentID, parseLimit(r, 50))
		if err != nil {
			if errors.Is(err, incidents.ErrIncidentNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list incident alert links")
			return
		}
		if shared.HasAssetRestriction(r.Context()) {
			groupAccess, authErr := d.accessibleGroupIDs(r.Context())
			if authErr != nil {
				writeAssetScopeForbidden(w, "unable to prove alert link asset scope")
				return
			}
			filtered := make([]incidents.AlertLink, 0, len(links))
			for _, link := range links {
				allowed, loadErr := d.incidentAlertLinkAllowed(r.Context(), link, groupAccess)
				if loadErr != nil {
					servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize alert links")
					return
				}
				if allowed {
					filtered = append(filtered, link)
				}
			}
			links = filtered
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"links": links})
		return
	}

	if len(parts) == 2 && parts[1] == "timeline" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if d.IncidentEventStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "incident event store unavailable")
			return
		}

		if _, ok, err := d.IncidentStore.GetIncident(incidentID); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load incident")
			return
		} else if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
			return
		}

		events, err := d.IncidentEventStore.ListIncidentEvents(incidentID, parseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list incident events")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"events": events})
		return
	}

	if len(parts) == 3 && parts[1] == "unlink-alert" {
		if r.Method != http.MethodDelete {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		linkID := strings.TrimSpace(parts[2])
		if linkID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "link_id is required")
			return
		}
		if err := d.IncidentStore.UnlinkIncidentAlert(incidentID, linkID); err != nil {
			if errors.Is(err, incidents.ErrIncidentNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
				return
			}
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "alert link not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to unlink alert")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"unlinked": true, "link_id": linkID})
		return
	}

	if len(parts) == 2 && parts[1] == "link-asset" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if d.DependencyStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "dependency store unavailable")
			return
		}
		if !d.EnforceRateLimit(w, r, "incidents.link_asset", 240, time.Minute) {
			return
		}

		var req incidents.LinkAssetRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid link asset payload")
			return
		}
		req.AssetID = strings.TrimSpace(req.AssetID)
		req.Role = strings.TrimSpace(req.Role)
		if req.AssetID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "asset_id is required")
			return
		}
		if !apiv2.RequireAssetAccess(w, r, req.AssetID) {
			return
		}
		if incidents.NormalizeAssetRole(req.Role) == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "role must be primary, impacted, related, or contributing")
			return
		}
		if _, ok, err := d.AssetStore.GetAsset(req.AssetID); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate asset")
			return
		} else if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "asset not found")
			return
		}

		ia, err := d.DependencyStore.LinkIncidentAsset(incidentID, req)
		if err != nil {
			if errors.Is(err, incidents.ErrIncidentNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
				return
			}
			if errors.Is(err, incidents.ErrIncidentAssetConflict) {
				servicehttp.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to link asset")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"incident_asset": ia})
		return
	}

	if len(parts) == 2 && parts[1] == "assets" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if d.DependencyStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "dependency store unavailable")
			return
		}
		assets, err := d.DependencyStore.ListIncidentAssets(incidentID, parseLimit(r, 50))
		if err != nil {
			if errors.Is(err, incidents.ErrIncidentNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list incident assets")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"assets": assets})
		return
	}

	if len(parts) == 3 && parts[1] == "unlink-asset" {
		if r.Method != http.MethodDelete {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if d.DependencyStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "dependency store unavailable")
			return
		}
		linkID := strings.TrimSpace(parts[2])
		if linkID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "link_id is required")
			return
		}
		if err := d.DependencyStore.UnlinkIncidentAsset(incidentID, linkID); err != nil {
			if errors.Is(err, incidents.ErrIncidentNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
				return
			}
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "asset link not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to unlink asset")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"unlinked": true, "link_id": linkID})
		return
	}

	if len(parts) == 2 && parts[1] == "export" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		incident, ok, err := d.IncidentStore.GetIncident(incidentID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load incident")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
			return
		}

		var alertLinks []incidents.AlertLink
		links, err := d.IncidentStore.ListIncidentAlertLinks(incidentID, 100)
		if err == nil {
			alertLinks = links
		}

		md := buildIncidentPostmortem(incident, alertLinks)
		w.Header().Set("Content-Type", "text/markdown")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="incident-%s-postmortem.md"`, incidentID))
		w.WriteHeader(http.StatusOK)
		// #nosec G705 -- markdown is returned as attachment text, not rendered HTML.
		_, _ = w.Write([]byte(md))
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "unknown incident action")
}
