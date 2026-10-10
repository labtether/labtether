package alerting

import (
	"errors"
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

func (d *Deps) HandleIncidents(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/incidents" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.IncidentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "incident store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		groupID := groupIDQueryParam(r)
		if groupID != "" {
			if d.GroupStore == nil {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
				return
			}
			if _, ok, err := d.GroupStore.GetGroup(groupID); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load group")
				return
			} else if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "group not found")
				return
			}
			if shared.HasAssetRestriction(r.Context()) {
				groupAccess, authErr := d.accessibleGroupIDs(r.Context())
				if authErr != nil {
					writeAssetScopeForbidden(w, "unable to prove incident group scope")
					return
				}
				if _, allowed := groupAccess[groupID]; !allowed {
					writeAssetScopeForbidden(w, "api key does not have access to every asset in this incident group")
					return
				}
			}
		}

		filter := persistence.IncidentFilter{
			Limit:    parseLimit(r, 50),
			Offset:   parseOffset(r),
			Status:   r.URL.Query().Get("status"),
			Severity: r.URL.Query().Get("severity"),
			GroupID:  groupID,
			Assignee: r.URL.Query().Get("assignee"),
			Source:   r.URL.Query().Get("source"),
		}
		var listed []incidents.Incident
		var err error
		if shared.HasAssetRestriction(r.Context()) {
			groupAccess, authErr := d.accessibleGroupIDs(r.Context())
			if authErr != nil {
				writeAssetScopeForbidden(w, "unable to prove incident asset scope")
				return
			}
			listed, err = d.listAccessibleIncidents(r.Context(), filter, groupAccess)
		} else {
			listed, err = d.IncidentStore.ListIncidents(filter)
		}
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list incidents")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"incidents": listed})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "incidents.create", 120, time.Minute) {
			return
		}

		var req incidents.CreateIncidentRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid incident payload")
			return
		}
		normalizeCreateIncidentRequest(&req)
		req.CreatedBy = apiv2.PrincipalActorID(r.Context())
		if err := validateCreateIncidentRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := d.validateIncidentReferences(req.GroupID, req.PrimaryAssetID); err != nil {
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
		if shared.HasAssetRestriction(r.Context()) {
			groupAccess, authErr := d.accessibleGroupIDs(r.Context())
			if authErr != nil || !incidentReferencesAllowed(r.Context(), req.GroupID, req.PrimaryAssetID, groupAccess) {
				writeAssetScopeForbidden(w, "api key may only create incidents scoped to explicitly allowed assets")
				return
			}
		}

		incident, err := d.IncidentStore.CreateIncident(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create incident")
			return
		}
		d.dispatchIncidentNotificationAsync(incident, "incident.created")
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"incident": incident})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
