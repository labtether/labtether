package alerting

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
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
		}

		listed, err := d.IncidentStore.ListIncidents(persistence.IncidentFilter{
			Limit:    parseLimit(r, 50),
			Offset:   parseOffset(r),
			Status:   r.URL.Query().Get("status"),
			Severity: r.URL.Query().Get("severity"),
			GroupID:  groupID,
			Assignee: r.URL.Query().Get("assignee"),
			Source:   r.URL.Query().Get("source"),
		})
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

		incident, err := d.IncidentStore.CreateIncident(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create incident")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"incident": incident})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

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
		if err := validateLinkAlertRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.AlertRuleID != "" {
			if _, ok, err := d.AlertStore.GetAlertRule(req.AlertRuleID); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load alert rule")
				return
			} else if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "alert rule not found")
				return
			}
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
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"links": links})
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

func buildIncidentPostmortem(inc incidents.Incident, alertLinks []incidents.AlertLink) string {
	var b strings.Builder
	const timeFmt = "2006-01-02 15:04:05 UTC"

	b.WriteString(fmt.Sprintf("# Incident: %s\n\n", inc.Title))

	// Summary
	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Severity**: %s\n", inc.Severity))
	b.WriteString(fmt.Sprintf("- **Status**: %s\n", inc.Status))
	b.WriteString(fmt.Sprintf("- **Source**: %s\n", inc.Source))
	if inc.Summary != "" {
		b.WriteString(fmt.Sprintf("\n%s\n", inc.Summary))
	}
	b.WriteString("\n")

	// Timeline
	b.WriteString("## Timeline\n\n")
	b.WriteString(fmt.Sprintf("- **Opened**: %s\n", inc.OpenedAt.UTC().Format(timeFmt)))
	if inc.MitigatedAt != nil {
		b.WriteString(fmt.Sprintf("- **Mitigated**: %s\n", inc.MitigatedAt.UTC().Format(timeFmt)))
	}
	if inc.ResolvedAt != nil {
		b.WriteString(fmt.Sprintf("- **Resolved**: %s\n", inc.ResolvedAt.UTC().Format(timeFmt)))
	}
	if inc.ClosedAt != nil {
		b.WriteString(fmt.Sprintf("- **Closed**: %s\n", inc.ClosedAt.UTC().Format(timeFmt)))
	}
	b.WriteString("\n")

	// Impact
	b.WriteString("## Impact\n\n")
	if inc.GroupID != "" {
		b.WriteString(fmt.Sprintf("- **Group**: %s\n", inc.GroupID))
	}
	if inc.PrimaryAssetID != "" {
		b.WriteString(fmt.Sprintf("- **Primary Asset**: %s\n", inc.PrimaryAssetID))
	}
	if inc.Assignee != "" {
		b.WriteString(fmt.Sprintf("- **Assignee**: %s\n", inc.Assignee))
	}
	b.WriteString("\n")

	// Root Cause
	b.WriteString("## Root Cause\n\n")
	if inc.RootCause != "" {
		b.WriteString(inc.RootCause + "\n")
	} else {
		b.WriteString("_Not yet documented._\n")
	}
	b.WriteString("\n")

	// Action Items
	b.WriteString("## Action Items\n\n")
	if len(inc.ActionItems) > 0 {
		for _, item := range inc.ActionItems {
			b.WriteString(fmt.Sprintf("- %s\n", item))
		}
	} else {
		b.WriteString("_No action items recorded._\n")
	}
	b.WriteString("\n")

	// Lessons Learned
	b.WriteString("## Lessons Learned\n\n")
	if inc.LessonsLearned != "" {
		b.WriteString(inc.LessonsLearned + "\n")
	} else {
		b.WriteString("_No lessons learned recorded._\n")
	}
	b.WriteString("\n")

	// Metrics
	b.WriteString("## Metrics\n\n")
	if inc.ResolvedAt != nil {
		mttr := inc.ResolvedAt.Sub(inc.OpenedAt)
		b.WriteString(fmt.Sprintf("- **MTTR (Mean Time to Resolve)**: %s\n", formatDuration(mttr)))
	} else {
		b.WriteString("_Incident not yet resolved — MTTR unavailable._\n")
	}
	b.WriteString("\n")

	// Linked Alerts
	b.WriteString("## Linked Alerts\n\n")
	if len(alertLinks) > 0 {
		for _, link := range alertLinks {
			label := link.AlertRuleID
			if label == "" {
				label = link.AlertFingerprint
			}
			if label == "" {
				label = link.AlertInstanceID
			}
			b.WriteString(fmt.Sprintf("- %s (type: %s)\n", label, link.LinkType))
		}
	} else {
		b.WriteString("_No linked alerts._\n")
	}
	b.WriteString("\n")

	return b.String()
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	days := hours / 24
	hours = hours % 24
	return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
}
