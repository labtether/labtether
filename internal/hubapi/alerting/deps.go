package alerting

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

// Validation length constants for alerting payloads.
const (
	MaxActorIDLength        = 64
	MaxAlertRuleNameLength  = 120
	MaxAlertDescriptionLen  = 2048
	MaxAlertTargetCount     = 200
	MaxIncidentTitleLength  = 160
	MaxIncidentSummaryLen   = 4096
	MaxIncidentLinkIDLength = 255
	MaxSilenceMatcherCount  = 16
	MaxSilenceMatcherKeyLen = 128
	MaxSilenceMatcherValLen = 512
)

// Deps holds all dependencies required by the alerting handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Store interfaces
	AlertStore         persistence.AlertStore
	AlertInstanceStore persistence.AlertInstanceStore
	IncidentStore      persistence.IncidentStore
	IncidentEventStore persistence.IncidentEventStore
	GroupStore         persistence.GroupStore
	AssetStore         persistence.AssetStore
	DependencyStore    persistence.DependencyStore
	NotificationStore  persistence.NotificationStore
	PushDeviceStore    PushDeviceStore
	LiveActivityStore  LiveActivityStore
	CanonicalStore     persistence.CanonicalModelStore
	TelemetryStore     persistence.TelemetryStore
	SyntheticStore     persistence.SyntheticStore
	LogStore           persistence.LogStore
	ActionStore        persistence.ActionStore
	UpdateStore        persistence.UpdateStore
	AuditStore         persistence.AuditStore

	// Notification runtime
	NotificationAdapters map[string]notifications.Adapter
	NotificationSem      chan struct{}
	NotificationWG       *sync.WaitGroup
	NotificationSecrets  NotificationSecretsManager
	// DispatchIncidentLiveActivity schedules ActivityKit delivery for material
	// incident transitions. It must return without performing network I/O.
	DispatchIncidentLiveActivity func(incident incidents.Incident, event string)
	// LiveActivityUserCanMutate revalidates current server-side role at send
	// time so a role downgrade immediately removes lock-screen actions.
	LiveActivityUserCanMutate func(userID string) bool

	// Auth and audit helpers injected from cmd/labtether.
	EnforceRateLimit           func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool
	PrincipalActorID           func(ctx context.Context) string
	AppendAuditEventBestEffort func(event audit.Event, logMessage string)

	// Event broadcaster (nil-safe; called from broadcastEvent).
	Broadcast func(eventType string, data map[string]any)

	// EvaluateGuardrails resolves active maintenance constraints for a group.
	EvaluateGuardrails func(groupID string, at time.Time) (groupfeatures.GroupMaintenanceGuardrails, error)

	// Agent manager for push notifications.
	AgentMgr *agentmgr.AgentManager

	// Canonical helpers injected from cmd/labtether.
	InferCapabilityIDsFromAssetMetadata func(entry assets.Asset) []string
	CapabilityIDsFromSet                func(set model.CapabilitySet) []string
	MergeCapabilityIDs                  func(values ...[]string) []string

	// Web service template constants (injected so alerting templates can reference them).
	WebServiceHealthLogSource      string
	WebServiceStatusTransitionKind string
	WebServiceUptimeDropKind       string
	WebServiceUptimeDropThreshold  float64

	// WrapAuth / WrapAdmin for route registration.
	WrapAuth  func(http.HandlerFunc) http.HandlerFunc
	WrapAdmin func(http.HandlerFunc) http.HandlerFunc
}

// NotificationSecretsManager is the narrow encryption contract needed for
// notification-channel configuration. Ciphertext is bound to a channel and
// field path through per-value AAD.
type NotificationSecretsManager interface {
	EncryptString(plaintext, aad string) (string, error)
	DecryptString(ciphertext, aad string) (string, error)
}

// PushDeviceStore is the narrow persistence contract needed for APNs fanout.
type PushDeviceStore interface {
	GetAllPushTokens(ctx context.Context) ([]persistence.PushDevice, error)
	DeletePushDeviceByToken(ctx context.Context, pushToken, bundleID, environment string) error
}

// LiveActivityStore persists encrypted, expiring ActivityKit delivery tokens
// and bounded retry state. Implementations must never return tokens via API
// responses or logs.
type LiveActivityStore interface {
	UpsertLiveActivityPushToken(context.Context, persistence.LiveActivityPushToken) error
	DeleteLiveActivityPushToken(context.Context, string, string, string, string) error
	DeleteLiveActivityPushTokenByOwnerAndID(context.Context, string, string, string, string, string) error
	DeleteLiveActivityPushTokenByID(context.Context, string) error
	DeleteLiveActivityPushTokenByGeneration(context.Context, string, int64) error
	ListLiveActivityPushTokensForIncident(context.Context, string, time.Time) ([]persistence.LiveActivityPushToken, error)
	ListDueLiveActivityPushTokens(context.Context, time.Time, int) ([]persistence.LiveActivityPushToken, error)
	ListLiveActivityPushTokensForReconciliation(context.Context, time.Time, int) ([]persistence.LiveActivityPushToken, error)
	ClaimLiveActivityPushDelivery(context.Context, string, int64, string, time.Time, time.Time, int) (int64, bool, error)
	MarkLiveActivityPushRetry(context.Context, string, int64, int, time.Time, string) error
	ClearLiveActivityPushRetry(context.Context, string, int64, time.Time) error
	DeleteExpiredLiveActivityPushTokens(context.Context, time.Time) error
}

// RegisterRoutes registers all alerting, incident, and notification API routes on the given mux.
func RegisterRoutes(mux *http.ServeMux, d *Deps) {
	mux.HandleFunc("/alerts/rules", d.WrapAuth(d.HandleAlertRules))
	mux.HandleFunc("/alerts/rules/", d.WrapAuth(d.HandleAlertRuleActions))
	mux.HandleFunc("/alerts/instances", d.WrapAuth(d.HandleAlertInstances))
	mux.HandleFunc("/alerts/instances/", d.WrapAuth(d.HandleAlertInstanceActions))
	mux.HandleFunc("/alerts/silences", d.WrapAuth(d.HandleAlertSilences))
	mux.HandleFunc("/alerts/silences/", d.WrapAuth(d.HandleAlertSilenceActions))
	mux.HandleFunc("/alerts/templates", d.WrapAuth(d.HandleAlertTemplates))
	mux.HandleFunc("/alerts/templates/", d.WrapAuth(d.HandleAlertTemplateActions))
	mux.HandleFunc("/alerts/routes", d.WrapAuth(d.HandleAlertRoutes))
	mux.HandleFunc("/alerts/routes/", d.WrapAuth(d.HandleAlertRouteActions))

	mux.HandleFunc("/incidents", d.WrapAuth(d.HandleIncidents))
	mux.HandleFunc("/incidents/", d.WrapAuth(d.HandleIncidentActions))
	mux.HandleFunc("/live-activities/incidents/", d.WrapAuth(d.HandleIncidentLiveActivityTokens))

	mux.HandleFunc("/notifications/channels", d.WrapAuth(d.HandleNotificationChannels))
	mux.HandleFunc("/notifications/channels/", d.WrapAuth(d.RouteNotificationChannelActions))
	mux.HandleFunc("/notifications/history", d.WrapAuth(d.HandleNotificationHistory))
}

// broadcastEvent calls the Broadcast function if non-nil.
func (d *Deps) broadcastEvent(eventType string, data map[string]any) {
	if d.Broadcast != nil {
		d.Broadcast(eventType, data)
	}
}

func (d *Deps) principalActorID(ctx context.Context) string {
	if d.PrincipalActorID != nil {
		return d.PrincipalActorID(ctx)
	}
	return ""
}

func (d *Deps) appendAuditEventBestEffort(event audit.Event, logMessage string) {
	if d.AppendAuditEventBestEffort != nil {
		d.AppendAuditEventBestEffort(event, logMessage)
	}
}

// inferCapabilityIDsFromAssetMetadata delegates to the injected function.
func (d *Deps) inferCapabilityIDsFromAssetMetadata(entry assets.Asset) []string {
	if d.InferCapabilityIDsFromAssetMetadata != nil {
		return d.InferCapabilityIDsFromAssetMetadata(entry)
	}
	return nil
}

// capabilityIDsFromSet delegates to the injected function.
func (d *Deps) capabilityIDsFromSet(set model.CapabilitySet) []string {
	if d.CapabilityIDsFromSet != nil {
		return d.CapabilityIDsFromSet(set)
	}
	return nil
}

// mergeCapabilityIDs delegates to the injected function.
func (d *Deps) mergeCapabilityIDs(values ...[]string) []string {
	if d.MergeCapabilityIDs != nil {
		return d.MergeCapabilityIDs(values...)
	}
	// Simple fallback: concatenate.
	var out []string
	for _, v := range values {
		out = append(out, v...)
	}
	return out
}

// --- Thin helper aliases delegating to shared ---

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	return shared.DecodeJSONBody(w, r, dst)
}

func parseLimit(r *http.Request, fallback int) int { return shared.ParseLimit(r, fallback) }

func parseOffset(r *http.Request) int { return shared.ParseOffset(r) }

func groupIDQueryParam(r *http.Request) string { return shared.GroupIDQueryParam(r) }

func validateMaxLen(field, value string, maxLen int) error {
	return shared.ValidateMaxLen(field, value, maxLen)
}

func cloneAnyMap(input map[string]any) map[string]any { return shared.CloneAnyMap(input) }

// --- Local helpers ---

func cloneMetadata(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func trimStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
