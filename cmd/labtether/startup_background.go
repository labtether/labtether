package main

import (
	"context"
	"log"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// safeGo starts a named background loop with panic recovery. Production
// always uses servicehttp.SafeGo; the wiring pin test replaces it to record
// the loop names startRuntimeLoops starts without running them.
var safeGo = servicehttp.SafeGo

func startRuntimeLoops(
	ctx context.Context,
	srv *apiServer,
	pgStore *persistence.PostgresStore,
	workerState *workerRuntimeState,
	retentionTracker *retentionState,
) {
	// Recovery: any persistent sessions that were "attached" when the hub died
	// should be marked "detached" so the archive worker can eventually clean them up.
	if srv.terminalPersistentStore != nil {
		if err := srv.terminalPersistentStore.MarkAllAttachedAsDetached(); err != nil {
			log.Printf("labtether: warning: startup recovery for attached persistent sessions failed: %v", err)
		}
	}

	wg := &srv.backgroundWG

	safeGo(ctx, wg, "heartbeat", func(ctx context.Context) { heartbeatLoop(ctx) })
	safeGo(ctx, wg, "retention", func(ctx context.Context) { runRetentionLoop(ctx, pgStore, workerState, retentionTracker) })
	startMetricsExport(ctx, srv)
	startPrometheusRemoteWrite(ctx, srv, pgStore)
	safeGo(ctx, wg, "session-cleanup", func(ctx context.Context) { runSessionCleanupLoop(ctx, pgStore) })
	safeGo(ctx, wg, "alert-evaluator", func(ctx context.Context) { srv.ensureAlertingDeps().RunAlertEvaluator(ctx) })
	safeGo(ctx, wg, "synthetic-runner", func(ctx context.Context) { srv.runSyntheticRunner(ctx) })
	safeGo(ctx, wg, "incident-correlator", func(ctx context.Context) { srv.ensureAlertingDeps().RunIncidentCorrelator(ctx) })
	safeGo(ctx, wg, "hub-collector", func(ctx context.Context) { srv.ensureCollectorsDeps().RunHubCollectorLoop(ctx) })
	safeGo(ctx, wg, "presence-cleanup", func(ctx context.Context) { srv.runPresenceCleanup(ctx) })
	safeGo(ctx, wg, "web-service-cleanup", func(ctx context.Context) { srv.ensureCollectorsDeps().RunWebServiceCleanup(ctx) })
	safeGo(ctx, wg, "notification-retry", func(ctx context.Context) { srv.ensureAlertingDeps().RunNotificationRetryLoop(ctx) })
	if srv.demoMode {
		safeGo(ctx, wg, "demo-fleet", func(ctx context.Context) { srv.runDemoFleet(ctx) })
	} else {
		safeGo(ctx, wg, "schedule-runner", func(ctx context.Context) { srv.runScheduleRunner(ctx) })
	}
	safeGo(ctx, wg, "live-activity-dispatch", func(ctx context.Context) { srv.runLiveActivityDispatchLoop(ctx) })
	safeGo(ctx, wg, "live-activity-retry", func(ctx context.Context) { srv.ensureAlertingDeps().RunLiveActivityRetryLoop(ctx) })
	safeGo(ctx, wg, "protocol-health", func(ctx context.Context) { srv.runProtocolHealthChecker(ctx) })
	safeGo(ctx, wg, "service-health-linker", func(ctx context.Context) { srv.runServiceHealthLinker(ctx) })
	if !srv.demoMode {
		// Demo mode delivers no webhooks.
		safeGo(ctx, wg, "webhook-relay", func(ctx context.Context) { srv.runWebhookRelay(ctx) })
	}
	if srv.failoverStore != nil && srv.groupStore != nil && srv.assetStore != nil {
		safeGo(ctx, wg, "failover-readiness", func(ctx context.Context) { srv.runFailoverReadinessChecker(ctx) })
	}
	if pgStore != nil && srv.groupStore != nil && srv.assetStore != nil {
		groupFeaturesDeps := srv.ensureGroupFeaturesDeps()
		safeGo(ctx, wg, "reliability-materializer", func(ctx context.Context) {
			groupFeaturesDeps.RunReliabilityMaterializer(ctx, pgStore)
		})
	}

	// Archive worker: periodically archives stale detached persistent sessions.
	terminalDeps := srv.ensureTerminalDeps()
	safeGo(ctx, wg, "archive-worker", func(ctx context.Context) { terminalDeps.StartArchiveWorker(ctx) })

	// Bounded API key touch drainer — drains apiKeyTouchCh sequentially.
	safeGo(ctx, wg, "apikey-touch", func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case keyID := <-srv.apiKeyTouchCh:
				touchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				if err := srv.apiKeyStore.TouchAPIKeyLastUsed(touchCtx, keyID); err != nil {
					log.Printf("apikey: touch last-used for %s: %v", keyID, err)
				}
				cancel()
			}
		}
	})

	// Bounded request-audit drainer. HTTP handlers enqueue without blocking;
	// one worker serializes store writes and prevents request floods from
	// spawning unbounded persistence goroutines.
	safeGo(ctx, wg, "request-audit", func(ctx context.Context) {
		runRequestAuditWorker(ctx, srv)
	})

	// Send graceful shutdown to connected agents when the hub exits.
	go func() {
		<-ctx.Done()
		srv.ensureAgentsDeps().SendShutdownToAgents()
	}()
}

func runRequestAuditWorker(ctx context.Context, srv *apiServer) {
	if srv == nil || srv.auditEventCh == nil {
		<-ctx.Done()
		return
	}
	appendEvent := func(event audit.Event) {
		if srv.auditStore == nil {
			return
		}
		if err := srv.auditStore.Append(event); err != nil {
			log.Printf("audit: failed to append event: %v", err)
		}
	}

	for {
		select {
		case event := <-srv.auditEventCh:
			appendEvent(event)
		case <-ctx.Done():
			// HTTP shutdown has completed before the runtime context is canceled.
			// Drain what was already accepted so shutdown does not silently lose
			// bounded request-audit events, then terminate without closing a channel
			// that request handlers may still reference.
			for {
				select {
				case event := <-srv.auditEventCh:
					appendEvent(event)
				default:
					return
				}
			}
		}
	}
}
