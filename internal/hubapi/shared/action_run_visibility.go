package shared

import (
	"context"
	"strings"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
)

// SavedActionRunParam marks an action run that came from a person's saved
// action. Saved actions are private to the person who saved them, so their
// runs are too.
const SavedActionRunParam = "saved_action_id"

// ActionRunVisible reports whether the request's principal may see a run.
// Runs of a saved action show only to the person who ran it and to the
// owner, like terminal commands; every other run keeps its existing access.
func ActionRunVisible(ctx context.Context, run actions.Run) bool {
	if strings.TrimSpace(run.Params[SavedActionRunParam]) == "" || apiv2.IsOwnerPrincipal(ctx) {
		return true
	}
	return strings.TrimSpace(run.ActorID) == strings.TrimSpace(apiv2.PrincipalActorID(ctx))
}

// VisibleActionRuns keeps the runs ActionRunVisible allows, in order.
func VisibleActionRuns(ctx context.Context, runs []actions.Run) []actions.Run {
	out := make([]actions.Run, 0, len(runs))
	for _, run := range runs {
		if ActionRunVisible(ctx, run) {
			out = append(out, run)
		}
	}
	return out
}
