package main

import (
	"context"
	"net/http"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	alertingpkg "github.com/labtether/labtether/internal/hubapi/alerting"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/notifications"
)

// buildAlertingDeps constructs the alerting.Deps from the apiServer's fields.
func (s *apiServer) buildAlertingDeps() *alertingpkg.Deps {
	return &alertingpkg.Deps{
		AlertStore:         s.alertStore,
		AlertInstanceStore: s.alertInstanceStore,
		IncidentStore:      s.incidentStore,
		IncidentEventStore: s.incidentEventStore,
		GroupStore:         s.groupStore,
		AssetStore:         s.assetStore,
		DependencyStore:    s.dependencyStore,
		NotificationStore:  s.notificationStore,
		CanonicalStore:     s.canonicalStore,
		TelemetryStore:     s.telemetryStore,
		SyntheticStore:     s.syntheticStore,
		LogStore:           s.logStore,
		ActionStore:        s.actionStore,
		UpdateStore:        s.updateStore,
		AuditStore:         s.auditStore,

		NotificationAdapters: s.notificationDispatcher.Adapters,
		NotificationSem:      s.notificationDispatcher.DispatchSem,
		NotificationWG:       &s.notificationDispatcher.DispatchWG,

		EnforceRateLimit: s.enforceRateLimit,

		Broadcast: func(eventType string, data map[string]any) {
			if s.broadcaster != nil {
				s.broadcaster.Broadcast(eventType, data)
			}
		},

		AgentMgr: s.agentMgr,

		InferCapabilityIDsFromAssetMetadata: func(entry assets.Asset) []string {
			return inferCapabilityIDsFromAssetMetadata(entry)
		},
		CapabilityIDsFromSet: func(set model.CapabilitySet) []string {
			return capabilityIDsFromSet(set)
		},
		MergeCapabilityIDs: func(values ...[]string) []string {
			return mergeCapabilityIDs(values...)
		},

		WebServiceHealthLogSource:      webServiceHealthLogSource,
		WebServiceStatusTransitionKind: webServiceStatusTransitionKind,
		WebServiceUptimeDropKind:       webServiceUptimeDropKind,
		WebServiceUptimeDropThreshold:  webServiceUptimeDropThreshold,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,
	}
}

// ensureAlertingDeps returns the alerting deps. When pre-initialized (production),
// returns the cached instance. Otherwise, rebuilds on every call so that test
// mutations to apiServer fields are visible.
//
// Like ensureProxmoxDeps, this pattern does NOT have the race-on-lazy-init
// fixed in ensureCollectorsDeps, because neither branch mutates shared state.
func (s *apiServer) ensureAlertingDeps() *alertingpkg.Deps {
	if s.alertingDeps != nil {
		return s.alertingDeps
	}
	return s.buildAlertingDeps()
}

// --- Forwarding methods from apiServer to alerting.Deps ---

func (s *apiServer) handleAlertRules(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleAlertRules(w, r)
}

func (s *apiServer) handleAlertRuleActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleAlertRuleActions(w, r)
}

func (s *apiServer) handleAlertInstances(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleAlertInstances(w, r)
}

func (s *apiServer) handleAlertInstanceActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleAlertInstanceActions(w, r)
}

func (s *apiServer) handleAlertSilences(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleAlertSilences(w, r)
}

func (s *apiServer) handleAlertSilenceActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleAlertSilenceActions(w, r)
}

func (s *apiServer) handleAlertTemplates(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleAlertTemplates(w, r)
}

func (s *apiServer) handleAlertTemplateActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleAlertTemplateActions(w, r)
}

func (s *apiServer) handleAlertRoutes(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleAlertRoutes(w, r)
}

func (s *apiServer) handleAlertRouteActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleAlertRouteActions(w, r)
}

func (s *apiServer) handleIncidents(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleIncidents(w, r)
}

func (s *apiServer) handleIncidentActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleIncidentActions(w, r)
}

func (s *apiServer) handleNotificationChannels(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleNotificationChannels(w, r)
}

func (s *apiServer) handleNotificationChannelActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().RouteNotificationChannelActions(w, r)
}

func (s *apiServer) handleNotificationHistory(w http.ResponseWriter, r *http.Request) {
	s.ensureAlertingDeps().HandleNotificationHistory(w, r)
}

// Background loop forwarding.

func (s *apiServer) runAlertEvaluator(ctx context.Context) {
	s.ensureAlertingDeps().RunAlertEvaluator(ctx)
}

func (s *apiServer) runIncidentCorrelator(ctx context.Context) {
	s.ensureAlertingDeps().RunIncidentCorrelator(ctx)
}

func (s *apiServer) runNotificationRetryLoop(ctx context.Context) {
	s.ensureAlertingDeps().RunNotificationRetryLoop(ctx)
}

func (s *apiServer) waitForNotificationDispatches() {
	s.ensureAlertingDeps().WaitForNotificationDispatches()
}

// Forwarding methods used by tests.

func (s *apiServer) resolveRuleTargetAssets(rule alerts.Rule, prefetchedAssets []assets.Asset, allowGlobalFallback bool) ([]assets.Asset, error) {
	return s.ensureAlertingDeps().ResolveRuleTargetAssets(rule, prefetchedAssets, allowGlobalFallback)
}

func (s *apiServer) evaluateMetricThreshold(ctx context.Context, rule alerts.Rule) (bool, error) {
	return s.ensureAlertingDeps().EvaluateMetricThreshold(ctx, rule)
}

func (s *apiServer) validateAlertRuleTargets(targets []alerts.RuleTargetInput) error {
	return s.ensureAlertingDeps().ValidateAlertRuleTargets(targets)
}

func (s *apiServer) evaluateSingleRule(ctx context.Context, rule alerts.Rule, prefetch *alertingpkg.AlertEvaluationPrefetch) {
	s.ensureAlertingDeps().EvaluateSingleRule(ctx, rule, prefetch)
}

func (s *apiServer) buildAlertPredicateContext(rule alerts.Rule, prefetchedAssets []assets.Asset) (alertingpkg.AlertPredicateContext, error) {
	return s.ensureAlertingDeps().BuildAlertPredicateContext(rule, prefetchedAssets)
}

func (s *apiServer) dispatchAlertNotifications(rule alerts.Rule, instanceID, state string) {
	s.ensureAlertingDeps().DispatchAlertNotifications(rule, instanceID, state)
}

func (s *apiServer) routeMatchesAlert(route notifications.Route, rule alerts.Rule, state string, predicateContext alertingpkg.AlertPredicateContext) bool {
	return s.ensureAlertingDeps().RouteMatchesAlert(route, rule, state, predicateContext)
}

func (s *apiServer) maybeAutoCreateIncident(rule alerts.Rule, instance alerts.AlertInstance) {
	s.ensureAlertingDeps().MaybeAutoCreateIncident(rule, instance)
}

func (s *apiServer) evaluateMetricThresholdWithPrefetch(rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string) (bool, error) {
	return s.ensureAlertingDeps().EvaluateMetricThresholdWithPrefetch(rule, prefetchedAssets, prefetchedCapabilities)
}

func (s *apiServer) evaluateMetricDeadmanWithPrefetch(rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string) (bool, error) {
	return s.ensureAlertingDeps().EvaluateMetricDeadmanWithPrefetch(rule, prefetchedAssets, prefetchedCapabilities)
}

func (s *apiServer) evaluateLogPatternWithPrefetch(rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string) (bool, error) {
	return s.ensureAlertingDeps().EvaluateLogPatternWithPrefetch(rule, prefetchedAssets, prefetchedCapabilities)
}

// Standalone function forwarding for validation functions used by tests.

func normalizeCreateAlertRuleRequest(req *alerts.CreateRuleRequest) {
	alertingpkg.NormalizeCreateAlertRuleRequest(req)
}

func validateCreateAlertRuleRequest(req alerts.CreateRuleRequest) error {
	return alertingpkg.ValidateCreateAlertRuleRequest(req)
}

func normalizeUpdateAlertRuleRequest(req *alerts.UpdateRuleRequest) {
	alertingpkg.NormalizeUpdateAlertRuleRequest(req)
}

func validateUpdateAlertRuleRequest(existing alerts.Rule, req alerts.UpdateRuleRequest) error {
	return alertingpkg.ValidateUpdateAlertRuleRequest(existing, req)
}

// Type aliases for types used in cmd/labtether/ tests.
type alertRuleTemplateResponse = alertingpkg.AlertRuleTemplateResponse

func validateCreateChannelRequest(req notifications.CreateChannelRequest) error {
	return alertingpkg.ValidateCreateChannelRequest(req)
}

func validateCreateRouteRequest(req notifications.CreateRouteRequest) error {
	return alertingpkg.ValidateCreateRouteRequest(req)
}

func validateUpdateRouteRequest(req notifications.UpdateRouteRequest) error {
	return alertingpkg.ValidateUpdateRouteRequest(req)
}

func validateSilenceRequest(req alerts.CreateSilenceRequest) error {
	return alertingpkg.ValidateSilenceRequest(req)
}

// Canonical predicate validation forwarding.

var deprecatedCanonicalPredicateKeys = alertingpkg.DeprecatedCanonicalPredicateKeys

func validateNoDeprecatedCanonicalPredicateKeys(values map[string]any, fieldName string) error {
	return alertingpkg.ValidateNoDeprecatedCanonicalPredicateKeys(values, fieldName)
}
