package auth

import (
	"context"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// revokeStreams ends live streams that belong to credentials that just ended.
func (d *Deps) revokeStreams(keys ...string) {
	if d != nil && d.RevokeStreams != nil {
		d.RevokeStreams(keys...)
	}
}

// revokeUserStreams closes a user's streams and the streams opened with API
// keys that user created, because those keys lose the user's rights too.
func (d *Deps) revokeUserStreams(ctx context.Context, userID string) {
	keys := []string{shared.StreamActorKey(userID)}
	if d != nil && d.APIKeyStore != nil {
		if apiKeys, err := d.APIKeyStore.ListAPIKeys(ctx); err == nil {
			for _, key := range apiKeys {
				if key.CreatedBy == userID {
					keys = append(keys, shared.StreamActorKey("apikey:"+key.ID))
				}
			}
		}
	}
	d.revokeStreams(keys...)
}

// revokeUserStreamsIfDemoted closes the user's streams when a role change
// removes write or admin rights. Streams opened with the old rights must not
// outlive the change, whether an admin made it or SSO sign-in applied it.
func (d *Deps) revokeUserStreamsIfDemoted(ctx context.Context, userID, oldRole, newRole string) {
	lostWrite := auth.HasWritePrivileges(oldRole) && !auth.HasWritePrivileges(newRole)
	lostAdmin := auth.HasAdminPrivileges(oldRole) && !auth.HasAdminPrivileges(newRole)
	if lostWrite || lostAdmin {
		d.revokeUserStreams(ctx, userID)
	}
}
