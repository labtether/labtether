package main

import (
	"context"
	"log"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

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

	servicehttp.SafeGo(ctx, wg, "heartbeat", func(ctx context.Context) { heartbeatLoop(ctx) })
	servicehttp.SafeGo(ctx, wg, "retention", func(ctx context.Context) { runRetentionLoop(ctx, pgStore, workerState, retentionTracker) })
	startMetricsExport(ctx, srv)
	servicehttp.SafeGo(ctx, wg, "session-cleanup", func(ctx context.Context) { runSessionCleanupLoop(ctx, pgStore) })
	servicehttp.SafeGo(ctx, wg, "alert-evaluator", func(ctx context.Context) { srv.runAlertEvaluator(ctx) })
	servicehttp.SafeGo(ctx, wg, "synthetic-runner", func(ctx context.Context) { srv.runSyntheticRunner(ctx) })
	servicehttp.SafeGo(ctx, wg, "incident-correlator", func(ctx context.Context) { srv.runIncidentCorrelator(ctx) })
	servicehttp.SafeGo(ctx, wg, "hub-collector", func(ctx context.Context) { srv.runHubCollectorLoop(ctx) })
	servicehttp.SafeGo(ctx, wg, "presence-cleanup", func(ctx context.Context) { srv.runPresenceCleanup(ctx) })
	servicehttp.SafeGo(ctx, wg, "web-service-cleanup", func(ctx context.Context) { srv.runWebServiceCleanup(ctx) })
	servicehttp.SafeGo(ctx, wg, "notification-retry", func(ctx context.Context) { srv.runNotificationRetryLoop(ctx) })
	servicehttp.SafeGo(ctx, wg, "protocol-health", func(ctx context.Context) { srv.runProtocolHealthChecker(ctx) })
	servicehttp.SafeGo(ctx, wg, "service-health-linker", func(ctx context.Context) { srv.runServiceHealthLinker(ctx) })

	// Archive worker: periodically archives stale detached persistent sessions.
	terminalDeps := srv.ensureTerminalDeps()
	servicehttp.SafeGo(ctx, wg, "archive-worker", func(ctx context.Context) { terminalDeps.StartArchiveWorker(ctx) })

	// Bounded API key touch drainer — drains apiKeyTouchCh sequentially.
	servicehttp.SafeGo(ctx, wg, "apikey-touch", func(ctx context.Context) {
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

	// Send graceful shutdown to connected agents when the hub exits.
	go func() {
		<-ctx.Done()
		srv.sendShutdownToAgents()
	}()
}
