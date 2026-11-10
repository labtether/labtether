package resources

import (
	"net/http"
	"slices"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
)

const assetUpdatedAuditType = "asset.updated"

// auditAssetUpdated records a device that was renamed, moved to another group
// or given other tags, with what changed. An edit that changes nothing writes
// no row.
func (d *Deps) auditAssetUpdated(r *http.Request, before, after assets.Asset) {
	if d.AppendAuditEventBestEffort == nil {
		return
	}
	changed := []string{}
	details := map[string]any{}
	if before.Name != after.Name {
		changed = append(changed, "name")
		details["previous_name"] = before.Name
	}
	if before.GroupID != after.GroupID {
		changed = append(changed, "group")
		details["group_id"] = after.GroupID
		details["group_name"] = d.auditGroupName(after.GroupID)
		details["previous_group_name"] = d.auditGroupName(before.GroupID)
	}
	if !slices.Equal(before.Tags, after.Tags) {
		changed = append(changed, "tags")
		details["tags"] = after.Tags
	}
	if len(changed) == 0 {
		return
	}
	details["changed"] = changed
	event := audit.NewEvent(assetUpdatedAuditType)
	if d.PrincipalActorID != nil {
		event.ActorID = d.PrincipalActorID(r.Context())
	}
	event.Target = after.ID
	event.TargetName = strings.TrimSpace(after.Name)
	event.Decision = "applied"
	event.Details = details
	d.AppendAuditEventBestEffort(event, "api warning: failed to append device change audit event")
}

// auditGroupName is a group's name for an audit row, or "" for no group.
func (d *Deps) auditGroupName(groupID string) string {
	if d.GroupStore == nil || strings.TrimSpace(groupID) == "" {
		return ""
	}
	group, ok, err := d.GroupStore.GetGroup(groupID)
	if err != nil || !ok {
		return ""
	}
	return group.Name
}
