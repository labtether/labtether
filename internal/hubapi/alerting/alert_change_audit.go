package alerting

import (
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/notifications"
)

const (
	alertAcknowledgedAuditType   = "alert.acknowledged"
	alertResolvedAuditType       = "alert.resolved"
	alertDeletedAuditType        = "alert.deleted"
	alertSilenceCreatedAuditType = "alert.silence.created"
	alertSilenceDeletedAuditType = "alert.silence.deleted"
	alertRouteCreatedAuditType   = "alert.route.created"
	alertRouteUpdatedAuditType   = "alert.route.updated"
	alertRouteDeletedAuditType   = "alert.route.deleted"
	incidentCreatedAuditType     = "incident.created"
	incidentUpdatedAuditType     = "incident.updated"
	incidentDeletedAuditType     = "incident.deleted"
)

// auditChange records a change a person made, with the changed thing's name so
// the row still reads after the thing is gone.
func (d *Deps) auditChange(r *http.Request, eventType, targetID, targetName string, details map[string]any) {
	event := audit.NewEvent(eventType)
	event.ActorID = d.principalActorID(r.Context())
	event.Target = targetID
	event.TargetName = strings.TrimSpace(targetName)
	event.Decision = "applied"
	event.Details = details
	d.appendAuditEventBestEffort(event, "api warning: failed to append "+eventType+" audit event")
}

// auditAlertInstance records an acknowledged, resolved or deleted alert under
// its rule's name, which is what the alert list shows, and says which device
// the alert is for: one rule can fire on many devices.
func (d *Deps) auditAlertInstance(r *http.Request, eventType string, inst alerts.AlertInstance) {
	name := ""
	if d.AlertStore != nil && strings.TrimSpace(inst.RuleID) != "" {
		if rule, ok, err := d.AlertStore.GetAlertRule(inst.RuleID); err == nil && ok {
			name = rule.Name
		}
	}
	details := map[string]any{"rule_id": inst.RuleID, "severity": inst.Severity}
	d.addAlertDevices(details, inst)
	d.auditChange(r, eventType, inst.ID, name, details)
}

// maxAuditAlertDevices bounds the device list one alert row carries.
const maxAuditAlertDevices = 20

// addAlertDevices writes the alert's devices into an audit row's details, with
// their names as they are now, so the row still reads after a device is
// renamed or removed. One device: device_id and device_name. Several:
// device_count, and device_ids with device_names for the first 20 in the same
// order. A device that no longer exists keeps its ID and has no name.
func (d *Deps) addAlertDevices(details map[string]any, inst alerts.AlertInstance) {
	ids := alertInstanceDeviceIDs(inst)
	if len(ids) == 0 {
		return
	}
	deviceName := func(id string) string {
		if d.AssetStore == nil {
			return ""
		}
		entry, ok, err := d.AssetStore.GetAsset(id)
		if err != nil || !ok {
			return ""
		}
		return strings.TrimSpace(entry.Name)
	}
	if len(ids) == 1 {
		details["device_id"] = ids[0]
		if name := deviceName(ids[0]); name != "" {
			details["device_name"] = name
		}
		return
	}
	details["device_count"] = len(ids)
	ids = ids[:min(len(ids), maxAuditAlertDevices)]
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = deviceName(id)
	}
	details["device_ids"] = ids
	details["device_names"] = names
}

// alertInstanceDeviceIDs lists the devices an alert is for, the way the alert
// list does: the alert's own asset label first, then the devices it fired for.
func alertInstanceDeviceIDs(inst alerts.AlertInstance) []string {
	ids := []string{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	add(inst.Labels["asset_id"])
	if inst.FiringScope != nil {
		for _, id := range inst.FiringScope.AssetIDs {
			add(id)
		}
	}
	return ids
}

// auditAlertSilence names a silence by its reason, or by what it matches when
// no reason was given.
func (d *Deps) auditAlertSilence(r *http.Request, eventType string, silence alerts.AlertSilence) {
	name := strings.TrimSpace(silence.Reason)
	if name == "" {
		pairs := make([]string, 0, len(silence.Matchers))
		for key, value := range silence.Matchers {
			pairs = append(pairs, key+"="+value)
		}
		sort.Strings(pairs)
		name = strings.Join(pairs, ", ")
	}
	details := map[string]any{"matchers": silence.Matchers}
	if !silence.EndsAt.IsZero() {
		details["starts_at"] = silence.StartsAt.UTC().Format(time.RFC3339)
		details["ends_at"] = silence.EndsAt.UTC().Format(time.RFC3339)
	}
	d.auditChange(r, eventType, silence.ID, name, details)
}

func (d *Deps) auditAlertRoute(r *http.Request, eventType string, route notifications.Route) {
	d.auditChange(r, eventType, route.ID, route.Name, map[string]any{
		"enabled":         route.Enabled,
		"severity_filter": route.SeverityFilter,
		"channels":        len(route.ChannelIDs),
	})
}

func (d *Deps) auditIncident(r *http.Request, eventType string, incident incidents.Incident, details map[string]any) {
	d.auditChange(r, eventType, incident.ID, incident.Title, details)
}

// incidentUpdateAuditDetails lists what an edit changed. It carries the new
// status and owner, never the text of a note.
func incidentUpdateAuditDetails(before, after incidents.Incident) map[string]any {
	changed := []string{}
	note := func(field string, differs bool) {
		if differs {
			changed = append(changed, field)
		}
	}
	note("title", before.Title != after.Title)
	note("summary", before.Summary != after.Summary)
	note("status", before.Status != after.Status)
	note("severity", before.Severity != after.Severity)
	note("assignee", before.Assignee != after.Assignee)
	note("group", before.GroupID != after.GroupID)
	note("primary_asset", before.PrimaryAssetID != after.PrimaryAssetID)
	note("root_cause", before.RootCause != after.RootCause)
	note("action_items", !slices.Equal(before.ActionItems, after.ActionItems))
	note("lessons_learned", before.LessonsLearned != after.LessonsLearned)
	details := map[string]any{"changed": changed, "status": after.Status, "severity": after.Severity}
	if before.Assignee != after.Assignee {
		details["assignee"] = after.Assignee
	}
	if before.Title != after.Title {
		details["previous_name"] = before.Title
	}
	return details
}
