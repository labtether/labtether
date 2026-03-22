package groupfeatures

import "github.com/labtether/labtether/internal/persistence"

// Deps holds the dependencies for group feature handlers:
// reliability scoring, group timeline, maintenance windows, and drift detection.
type Deps struct {
	GroupStore            persistence.GroupStore
	AssetStore            persistence.AssetStore
	LogStore              persistence.LogStore
	ActionStore           persistence.ActionStore
	UpdateStore           persistence.UpdateStore
	GroupMaintenanceStore persistence.GroupMaintenanceStore
	GroupProfileStore     persistence.GroupProfileStore
}
