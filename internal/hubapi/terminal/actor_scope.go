package terminal

import (
	"context"
	"strings"
)

// requestActorID returns the stable authenticated principal used to isolate
// user-owned terminal state. API keys receive their own apikey:<id> principal,
// so they cannot read or mutate a browser user's preferences or saved content.
func (d *Deps) requestActorID(ctx context.Context) string {
	if d != nil && d.PrincipalActorID != nil {
		if actorID := strings.TrimSpace(d.PrincipalActorID(ctx)); actorID != "" {
			return actorID
		}
	}
	return "system"
}
