package statusagg

import (
	"context"
	"log"
	"sync/atomic"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/connectors/webservice"
	"github.com/labtether/labtether/internal/connectorsdk"
	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

// Deps holds all external dependencies required by the status aggregate
// package. All store interfaces are read-only from this package's perspective;
// the only mutations are cache updates on the embedded Cache field.
type Deps struct {
	// Store interfaces (read-only usage).
	AssetStore     persistence.AssetStore
	GroupStore     persistence.GroupStore
	TelemetryStore persistence.TelemetryStore
	LogStore       persistence.LogStore
	ActionStore    persistence.ActionStore
	UpdateStore    persistence.UpdateStore
	CanonicalStore persistence.CanonicalModelStore
	RuntimeStore   persistence.RuntimeSettingsStore
	AuditStore     persistence.AuditStore
	TerminalStore  persistence.TerminalStore

	// Connector registry for listing active connectors.
	ConnectorRegistry *connectorsdk.Registry

	// Web service coordinator for service up/total counts.
	WebServiceCoordinator *webservice.Coordinator

	// RetentionTracker provides the last retention error message for the
	// dashboard summary. Nil is safe — the error field will be empty.
	RetentionTracker *opspkg.RetentionState

	// ProcessedJobs is the atomic counter of processed worker jobs. Nil is safe.
	ProcessedJobs *atomic.Uint64

	// Cache holds all in-memory caching state for the status aggregate.
	// It is embedded by pointer so that cmd/labtether/server_types.go can
	// continue to embed StatusCache as a field and pass &s.statusCache here.
	Cache *StatusCache
}

// principalActorID returns the actor ID from the context. It delegates to the
// canonical apiv2 implementation so identity rules are consistent across all
// handler packages.
func principalActorID(ctx context.Context) string {
	return apiv2.PrincipalActorID(ctx)
}

// isOwnerActor reports whether actorID represents the owner principal.
func isOwnerActor(actorID string) bool {
	trimmed := actorID
	for len(trimmed) > 0 && trimmed[0] == ' ' {
		trimmed = trimmed[1:]
	}
	for len(trimmed) > 0 && trimmed[len(trimmed)-1] == ' ' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed == "owner"
}

// loadUpdatePlansByID bulk-loads update plans by ID and returns a map keyed
// by plan ID. It is the package-local equivalent of apiServer.loadUpdatePlansByID.
func (d *Deps) loadUpdatePlansByID(planIDs []string) (map[string]updates.Plan, error) {
	out := make(map[string]updates.Plan, len(planIDs))
	if d.UpdateStore == nil || len(planIDs) == 0 {
		return out, nil
	}
	for _, id := range planIDs {
		if _, exists := out[id]; exists {
			continue
		}
		plan, ok, err := d.UpdateStore.GetUpdatePlan(id)
		if err != nil {
			return out, err
		}
		if ok {
			out[id] = plan
		}
	}
	return out, nil
}

// updateRunTouchesGroup checks whether an update run's plan targets assets in
// the specified group. Results are memoised in planGroupCache.
func (d *Deps) updateRunTouchesGroup(
	planID, groupID string,
	assetGroup map[string]string,
	planGroupCache map[string]bool,
) (bool, error) {
	if groupID == "" {
		return true, nil
	}
	if cached, ok := planGroupCache[planID]; ok {
		return cached, nil
	}
	if d.UpdateStore == nil {
		return false, nil
	}
	plan, ok, err := d.UpdateStore.GetUpdatePlan(planID)
	if err != nil {
		return false, err
	}
	if !ok {
		planGroupCache[planID] = false
		return false, nil
	}
	touches := shared.UpdatePlanTouchesGroup(plan, groupID, assetGroup)
	planGroupCache[planID] = touches
	return touches, nil
}

// logf is a package-local logging shim that prefixes all messages with the
// component name for easy grep in production logs.
func logf(format string, args ...any) {
	log.Printf("status aggregate: "+format, args...)
}
