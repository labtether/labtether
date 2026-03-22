package main

import (
	"context"
	"log"

	"github.com/labtether/labtether/internal/persistence"
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

	go heartbeatLoop(ctx)
	go runRetentionLoop(ctx, pgStore, workerState, retentionTracker)
	startMetricsExport(ctx, srv)
	go runSessionCleanupLoop(ctx, pgStore)
	go srv.runAlertEvaluator(ctx)
	go srv.runSyntheticRunner(ctx)
	go srv.runIncidentCorrelator(ctx)
	go srv.runHubCollectorLoop(ctx)
	go srv.runPresenceCleanup(ctx)
	go srv.runWebServiceCleanup(ctx)
	go srv.runNotificationRetryLoop(ctx)
	go srv.runProtocolHealthChecker(ctx)
	go srv.runServiceHealthLinker(ctx)

	// Archive worker: periodically archives stale detached persistent sessions.
	terminalDeps := srv.ensureTerminalDeps()
	go terminalDeps.StartArchiveWorker(ctx)

	// Send graceful shutdown to connected agents when the hub exits.
	go func() {
		<-ctx.Done()
		srv.sendShutdownToAgents()
	}()
}
