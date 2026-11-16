package hubdeps

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/alerting"
	"github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

// NotificationDispatcher holds what cmd's apiServer.notificationDispatcher
// holds: the channel senders, the limit on sends in flight and the wait
// group for them. It lives on the Hub, so a send started through one
// alerting Deps is waited for through the next, as in cmd.
type NotificationDispatcher struct {
	Adapters    map[string]notifications.Adapter
	DispatchSem chan struct{}
	DispatchWG  sync.WaitGroup
}

// BuildAlertingDeps mirrors cmd's buildAlertingDeps field by field.
// Stand-ins replace cmd's route auth, rate limit, the live activity role
// check and the synthetic service owner lookup (both are cmd code). Like
// cmd's test server, the Hub has no database (the push device and live
// activity stores are its nil Postgres store), no event broadcaster and no
// live activity queue, so events and live activity updates are dropped as
// cmd drops them when it has none.
func (h *Hub) BuildAlertingDeps() *alerting.Deps {
	var db *persistence.PostgresStore
	return &alerting.Deps{
		AlertStore:         h.AlertStore,
		AlertInstanceStore: h.AlertInstanceStore,
		IncidentStore:      h.IncidentStore,
		IncidentEventStore: h.IncidentEventStore,
		GroupStore:         h.GroupStore,
		AssetStore:         h.AssetStore,
		DependencyStore:    h.DependencyStore,
		NotificationStore:  h.NotificationStore,
		PushDeviceStore:    db,
		LiveActivityStore:  db,
		CanonicalStore:     h.CanonicalStore,
		TelemetryStore:     h.TelemetryStore,
		SyntheticStore:     h.SyntheticStore,
		LogStore:           h.LogStore,
		ActionStore:        h.ActionStore,
		UpdateStore:        h.UpdateStore,
		AuditStore:         h.AuditStore,
		AgentAlertDevices:  h.agentAlertDevices(),

		NotificationAdapters:         h.NotificationDispatcher.Adapters,
		NotificationSem:              h.NotificationDispatcher.DispatchSem,
		NotificationWG:               &h.NotificationDispatcher.DispatchWG,
		NotificationSecrets:          h.SecretsManager,
		DispatchIncidentLiveActivity: func(incidents.Incident, string) {},
		LiveActivityUserCanMutate:    liveActivityRoleCheckNotTested(h.t),

		EnforceRateLimit: h.RateLimiter.Enforce,
		PrincipalActorID: apiv2.PrincipalActorID,
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			shared.AppendAuditEventBestEffort(h.AuditStore, event, logMessage)
		},

		Broadcast:                 func(string, map[string]any) {},
		EvaluateGuardrails:        h.GroupFeaturesDeps().EvaluateGuardrails,
		EvaluateGuardrailsContext: h.GroupFeaturesDeps().EvaluateGuardrailsContext,

		AgentMgr: h.AgentMgr,

		InferCapabilityIDsFromAssetMetadata: resources.InferCapabilityIDsFromAssetMetadata,
		CapabilityIDsFromSet:                resources.CapabilityIDsFromSet,
		MergeCapabilityIDs:                  resources.MergeCapabilityIDs,

		ResolveSyntheticServiceAsset:   syntheticServiceOwnerNotTested(h.t),
		WebServiceHealthLogSource:      collectors.WebServiceHealthLogSource,
		WebServiceStatusTransitionKind: collectors.WebServiceStatusTransitionKind,
		WebServiceUptimeDropKind:       collectors.WebServiceUptimeDropKind,
		WebServiceUptimeDropThreshold:  collectors.WebServiceUptimeDropThreshold,

		WrapAuth:  testutil.RouteAuthNotTested(h.t),
		WrapAdmin: testutil.RouteAuthNotTested(h.t),
	}
}

// AlertingDeps mirrors cmd's ensureAlertingDeps on a test server: a new
// build per call, so a test's store changes are seen at once.
func (h *Hub) AlertingDeps() *alerting.Deps {
	return h.BuildAlertingDeps()
}

// agentAlertDevices mirrors cmd's ensureAgentAlertDevices: one memory per
// Hub, kept across the per-call alerting Deps.
func (h *Hub) agentAlertDevices() *alerting.AgentAlertDevices {
	h.alertDevicesOnce.Do(func() { h.alertDevices = alerting.NewAgentAlertDevices() })
	return h.alertDevices
}

// notificationSendersNotTested has the channel types of cmd's
// defaultNotificationAdapters (webhook, email, Slack, APNs, ntfy, Gotify),
// whose real senders reach outside addresses. Each stand-in fails the test
// if a notification reaches it. Tests that check delivery set their own
// recording senders, as they do in cmd.
func notificationSendersNotTested(t *testing.T) map[string]notifications.Adapter {
	out := map[string]notifications.Adapter{}
	for _, typ := range []string{
		notifications.ChannelTypeWebhook,
		notifications.ChannelTypeEmail,
		notifications.ChannelTypeSlack,
		notifications.ChannelTypeAPNs,
		notifications.ChannelTypeNtfy,
		notifications.ChannelTypeGotify,
	} {
		out[typ] = notificationSenderNotTested{t: t, typ: typ}
	}
	return out
}

type notificationSenderNotTested struct {
	t   *testing.T
	typ string
}

func (a notificationSenderNotTested) Type() string { return a.typ }

func (a notificationSenderNotTested) Send(context.Context, map[string]any, map[string]any) error {
	a.t.Errorf("a notification reached the %s sender stand-in; package tests send nothing to real addresses", a.typ)
	return errors.New(a.typ + " sender is not wired in package tests")
}

// liveActivityRoleCheckNotTested stands in for the closure in cmd's
// buildAlertingDeps that rereads a user's role before a lock-screen action
// is offered. It fails the test if reached and answers "no", the safe side;
// that check is cmd's and is tested in cmd/labtether.
func liveActivityRoleCheckNotTested(t *testing.T) func(string) bool {
	return func(string) bool {
		t.Errorf("a live activity reached the role check stand-in; that check is tested in cmd/labtether")
		return false
	}
}

// syntheticServiceOwnerNotTested stands in for cmd's
// resolveSyntheticServiceAsset, which finds the one device that runs a
// service. It fails the test if reached and reports no owner; that lookup
// is cmd's and is tested in cmd/labtether.
func syntheticServiceOwnerNotTested(t *testing.T) func(context.Context, string) (assets.Asset, bool, error) {
	return func(context.Context, string) (assets.Asset, bool, error) {
		t.Errorf("a synthetic check reached the service owner stand-in; that lookup is tested in cmd/labtether")
		return assets.Asset{}, false, errors.New("service owner lookup is not wired in package tests")
	}
}
