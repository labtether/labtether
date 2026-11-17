package main

import (
	"context"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	alertingpkg "github.com/labtether/labtether/internal/hubapi/alerting"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/persistence"
)

type liveActivityDispatchJob struct {
	Incident incidents.Incident
	Event    string
}

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
		PushDeviceStore:    s.db,
		LiveActivityStore:  s.db,
		CanonicalStore:     s.canonicalStore,
		TelemetryStore:     s.telemetryStore,
		SyntheticStore:     s.syntheticStore,
		LogStore:           s.logStore,
		ActionStore:        s.actionStore,
		UpdateStore:        s.updateStore,
		AuditStore:         s.auditStore,
		AgentAlertDevices:  s.ensureAgentAlertDevices(),

		NotificationAdapters: s.notificationDispatcher.Adapters,
		NotificationSem:      s.notificationDispatcher.DispatchSem,
		NotificationWG:       &s.notificationDispatcher.DispatchWG,
		NotificationSecrets:  s.secretsManager,
		DispatchIncidentLiveActivity: func(incident incidents.Incident, event string) {
			if s.liveActivityDispatchCh == nil {
				return
			}
			// A bounded queue keeps incident mutation latency independent from APNs.
			// When saturated, the next material transition and foreground sync still
			// converge on current state without spawning unbounded goroutines.
			select {
			case s.liveActivityDispatchCh <- liveActivityDispatchJob{Incident: incident, Event: event}:
			default:
				// Preserve the newest material state (especially a terminal resolved
				// transition) by evicting one stale queued snapshot under saturation.
				select {
				case <-s.liveActivityDispatchCh:
				default:
				}
				select {
				case s.liveActivityDispatchCh <- liveActivityDispatchJob{Incident: incident, Event: event}:
				default:
				}
			}
		},
		LiveActivityUserCanMutate: func(userID string) bool {
			if s.authStore == nil {
				return false
			}
			user, ok, err := s.authStore.GetUserByID(userID)
			return err == nil && ok && auth.HasWritePrivileges(user.Role)
		},

		EnforceRateLimit:           s.enforceRateLimit,
		PrincipalActorID:           apiv2.PrincipalActorID,
		AppendAuditEventBestEffort: s.appendAuditEventBestEffort,

		Broadcast: func(eventType string, data map[string]any) {
			if s.broadcaster != nil {
				s.broadcaster.Broadcast(eventType, data)
			}
		},
		EvaluateGuardrails:        s.ensureGroupFeaturesDeps().EvaluateGuardrails,
		EvaluateGuardrailsContext: s.ensureGroupFeaturesDeps().EvaluateGuardrailsContext,

		AgentMgr: s.agentMgr,

		InferCapabilityIDsFromAssetMetadata: func(entry assets.Asset) []string {
			return respkg.InferCapabilityIDsFromAssetMetadata(entry)
		},
		CapabilityIDsFromSet: func(set model.CapabilitySet) []string {
			return respkg.CapabilityIDsFromSet(set)
		},
		MergeCapabilityIDs: func(values ...[]string) []string {
			return respkg.MergeCapabilityIDs(values...)
		},

		ResolveSyntheticServiceAsset:   s.resolveSyntheticServiceAsset,
		WebServiceHealthLogSource:      collectorspkg.WebServiceHealthLogSource,
		WebServiceStatusTransitionKind: collectorspkg.WebServiceStatusTransitionKind,
		WebServiceUptimeDropKind:       collectorspkg.WebServiceUptimeDropKind,
		WebServiceUptimeDropThreshold:  collectorspkg.WebServiceUptimeDropThreshold,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,
	}
}

func (s *apiServer) resolveSyntheticServiceAsset(ctx context.Context, serviceID string) (assets.Asset, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return assets.Asset{}, false, err
	}
	if s.webServiceCoordinator == nil || s.assetStore == nil {
		return assets.Asset{}, false, nil
	}
	store, ok := s.assetStore.(persistence.AssetContextStore)
	if !ok {
		return assets.Asset{}, false, nil
	}
	ownerID, found, err := s.webServiceCoordinator.ResolveServiceHost(ctx, serviceID)
	if err != nil || !found {
		return assets.Asset{}, false, err
	}
	entry, found, err := store.GetAssetContext(ctx, ownerID)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return assets.Asset{}, false, ctxErr
	}
	if err != nil || !found || entry.ID != ownerID {
		return assets.Asset{}, false, err
	}
	return entry, true, nil
}

// ensureAlertingDeps returns the alerting deps. When pre-initialized (production),
// returns the cached instance. Otherwise, rebuilds on every call so that test
// mutations to apiServer fields are visible.
//
// Like ensureProxmoxDeps, this pattern does NOT have the race-on-lazy-init
// fixed in ensureCollectorsDeps, because neither branch mutates shared state.
// ensureAgentAlertDevices returns the one memory of which agents were told
// an alert is firing. buildAlertingDeps runs on every use, so the memory
// lives on the server, not in a Deps value.
func (s *apiServer) ensureAgentAlertDevices() *alertingpkg.AgentAlertDevices {
	s.agentAlertDevicesOnce.Do(func() {
		if s.agentAlertDevices == nil {
			s.agentAlertDevices = alertingpkg.NewAgentAlertDevices()
		}
	})
	return s.agentAlertDevices
}

func (s *apiServer) ensureAlertingDeps() *alertingpkg.Deps {
	if s.alertingDeps != nil {
		return s.alertingDeps
	}
	return s.buildAlertingDeps()
}

// replayAlertsToAgentAsync sends a newly connected agent the alerts that are
// firing for its device (and those that ended in the last day), bounded by
// the connection and 30 seconds.
func (s *apiServer) replayAlertsToAgentAsync(conn *agentmgr.AgentConn) {
	deps := s.ensureAlertingDeps()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		go func() {
			select {
			case <-conn.Done():
				cancel()
			case <-ctx.Done():
			}
		}()
		deps.ReplayAlertsToAgent(ctx, conn)
	}()
}

// runLiveActivityDispatchLoop reads cmd's live activity queue and hands each
// job to the package.
func (s *apiServer) runLiveActivityDispatchLoop(ctx context.Context) {
	if s.liveActivityDispatchCh == nil {
		<-ctx.Done()
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.liveActivityDispatchCh:
			s.ensureAlertingDeps().DeliverIncidentLiveActivity(job.Incident, job.Event)
		}
	}
}
