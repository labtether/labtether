package modelregistry

import "github.com/labtether/labtether/internal/model"

var eventCatalog = []model.EventDescriptor{
	{ID: "event.log", Kind: model.EventKindLog, Severity: []string{"debug", "info", "warn", "error", "critical"}, Description: "Canonical normalized log record.", Attributes: []string{"source", "fields"}},
	{ID: "event.state_change", Kind: model.EventKindStateChange, Severity: []string{"info", "warn", "error"}, Description: "Resource state transition emitted during reconciliation.", Attributes: []string{"from", "to", "reason"}},
	{ID: "event.operation", Kind: model.EventKindOperation, Severity: []string{"info", "warn", "error"}, Description: "Operation lifecycle event.", Attributes: []string{"operation_id", "request_id", "status"}},
	{ID: "event.alert", Kind: model.EventKindAlert, Severity: []string{"warn", "error", "critical"}, Description: "Alert evaluation emission.", Attributes: []string{"rule_id", "instance_id", "severity"}},
	{ID: "event.incident", Kind: model.EventKindIncident, Severity: []string{"info", "warn", "error", "critical"}, Description: "Incident timeline event.", Attributes: []string{"incident_id", "phase"}},
	{ID: "event.topology", Kind: model.EventKindTopology, Severity: []string{"info", "warn"}, Description: "Relationship graph mutation or drift.", Attributes: []string{"relationship_id", "change"}},
	{ID: "event.connector", Kind: model.EventKindConnector, Severity: []string{"info", "warn", "error"}, Description: "Connector health and ingest status event.", Attributes: []string{"provider_instance_id", "stream", "status"}},
}

func EventCatalog() []model.EventDescriptor {
	if len(eventCatalog) == 0 {
		return nil
	}
	out := make([]model.EventDescriptor, len(eventCatalog))
	copy(out, eventCatalog)
	for idx := range out {
		out[idx].Severity = cloneStrings(out[idx].Severity)
		out[idx].TargetKinds = cloneStrings(out[idx].TargetKinds)
		out[idx].Attributes = cloneStrings(out[idx].Attributes)
	}
	return out
}
