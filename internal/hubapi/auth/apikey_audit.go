package auth

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/audit"
)

// auditAPIKeyUpdated records who changed an API key and what kind of change
// it was, under the name the key has now. Like the created and deleted rows,
// it never holds the key's secret, hash or prefix.
func (d *Deps) auditAPIKeyUpdated(r *http.Request, id string, details map[string]any) {
	if d.AppendAuditEventBestEffort == nil {
		return
	}
	name := ""
	if key, found, err := d.APIKeyStore.GetAPIKey(r.Context(), id); err == nil && found {
		name = key.Name
	}
	d.AppendAuditEventBestEffort(audit.Event{
		Type:       "api_key.updated",
		ActorID:    d.actorIDFromContext(r.Context()),
		Target:     id,
		TargetName: name,
		Details:    details,
		Timestamp:  time.Now().UTC(),
	}, "api key updated: "+id)
}
