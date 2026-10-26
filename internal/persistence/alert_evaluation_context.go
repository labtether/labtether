package persistence

import (
	"context"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/model"
)

// These additive capabilities keep the synchronous evaluator on its parent's
// context. It must report a missing capability rather than use a legacy read.
type AlertAssetContextStore interface {
	AssetContextStore
	ListAssetsContext(context.Context) ([]assets.Asset, error)
}

// AlertGroupContextStore keeps restricted manual-rule authorization cancellable.
type AlertGroupContextStore interface {
	ListGroupsContext(context.Context) ([]groups.Group, error)
}

type AlertCapabilityContextStore interface {
	GetCapabilitySetContext(context.Context, string, string) (model.CapabilitySet, bool, error)
	ListCapabilitySetsContext(context.Context, int) ([]model.CapabilitySet, error)
}

type AlertEvaluationContextStore interface {
	GetAlertRuleContext(context.Context, string) (alerts.Rule, bool, error)
	RecordAlertEvaluationContext(context.Context, string, alerts.Evaluation) (alerts.Evaluation, error)
}

type AlertInstanceContextStore interface {
	CreateAlertInstanceContext(context.Context, alerts.CreateInstanceRequest) (alerts.AlertInstance, error)
	GetActiveInstanceByFingerprintContext(context.Context, string, string) (alerts.AlertInstance, bool, error)
	ListAlertInstancesContext(context.Context, AlertInstanceFilter) ([]alerts.AlertInstance, error)
	UpdateAlertInstanceStatusContext(context.Context, string, string) (alerts.AlertInstance, error)
	UpdateAlertInstanceLastFiredContext(context.Context, string) error
	ListAlertSilencesContext(context.Context, int, bool) ([]alerts.AlertSilence, error)
}

type AlertIncidentContextStore interface {
	HasAutoIncidentForAlertInstanceContext(context.Context, string) (bool, error)
	CreateIncidentContext(context.Context, incidents.CreateIncidentRequest) (incidents.Incident, error)
	LinkIncidentAlertContext(context.Context, string, incidents.LinkAlertRequest) (incidents.AlertLink, error)
}
