package terminal

import (
	"net/http"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/terminal"
)

const (
	bookmarkCreatedAuditType = "terminal.bookmark.created"
	bookmarkUpdatedAuditType = "terminal.bookmark.updated"
	bookmarkDeletedAuditType = "terminal.bookmark.deleted"
)

// auditBookmark records who created, changed or deleted a terminal bookmark,
// with the bookmark's title so the row still reads after it is gone. Only
// where it points goes into the row, never the user name or a credential.
func (d *Deps) auditBookmark(r *http.Request, eventType string, bookmark terminal.Bookmark) {
	if d.AppendAuditEventBestEffort == nil {
		return
	}
	event := audit.NewEvent(eventType)
	if d.PrincipalActorID != nil {
		event.ActorID = d.PrincipalActorID(r.Context())
	}
	event.Target = bookmark.ID
	event.TargetName = bookmark.Title
	event.Decision = "applied"
	details := map[string]any{}
	if bookmark.AssetID != "" {
		details["asset_id"] = bookmark.AssetID
	}
	if bookmark.Host != "" {
		details["host"] = bookmark.Host
	}
	event.Details = details
	d.AppendAuditEventBestEffort(event, "api warning: failed to append terminal bookmark audit event")
}
