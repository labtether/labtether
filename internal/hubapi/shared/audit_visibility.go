package shared

import (
	"context"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
)

// AuditReaderSeesAll reports whether the principal may read every audit row:
// the owner, admins, and API keys whose role is admin. Everyone else reads
// only the rows they caused (owner decision, 2026-11-10).
func AuditReaderSeesAll(ctx context.Context) bool {
	role := apiv2.UserRoleFromContext(ctx)
	if apiv2.IsAPIKeyPrincipal(ctx) {
		// A key is never the owner; only an admin key reads everything.
		return auth.NormalizeRole(role) == auth.RoleAdmin
	}
	return auth.HasAdminPrivileges(role)
}

// ListAuditEventsFor returns the newest audit rows the principal may read.
// A principal without an ID reads nothing, never the Hub's own rows.
func ListAuditEventsFor(ctx context.Context, store persistence.AuditStore, limit, offset int) ([]audit.Event, error) {
	if AuditReaderSeesAll(ctx) {
		return store.List(limit, offset)
	}
	actorID := strings.TrimSpace(apiv2.UserIDFromContext(ctx))
	if actorID == "" {
		return []audit.Event{}, nil
	}
	if byActor, ok := store.(persistence.AuditActorStore); ok {
		return byActor.ListByActor(actorID, limit, offset)
	}
	// A store without the per-actor query: filter what it returns. Pages can
	// come back short, but no other person's row gets through.
	all, err := store.List(limit, offset)
	if err != nil {
		return nil, err
	}
	own := make([]audit.Event, 0, len(all))
	for _, event := range all {
		if strings.TrimSpace(event.ActorID) == actorID {
			own = append(own, event)
		}
	}
	return own, nil
}
