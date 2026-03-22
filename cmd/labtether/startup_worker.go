package main

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/retention"
)

type workerBootstrap struct {
	state            *workerRuntimeState
	retentionTracker *retentionState
	counters         *workerCounters
}

func initializeWorkerSubsystem(ctx context.Context, srv *apiServer, pgStore *persistence.PostgresStore) workerBootstrap {
	counters := &workerCounters{}

	maxDeliveries := envOrDefaultUint64("QUEUE_MAX_DELIVERIES", 5)
	retentionInterval := envOrDefaultDuration("RETENTION_APPLY_INTERVAL", 5*time.Minute)
	workerState := newWorkerRuntimeState(maxDeliveries, retentionInterval)

	retentionTracker := &retentionState{
		Settings: retention.DefaultSettings(),
	}

	// Postgres job queue (replaces NATS for job dispatch).
	pollInterval := envOrDefaultDuration("JOB_POLL_INTERVAL", 500*time.Millisecond)
	jq := jobqueue.New(pgStore.Pool(), pollInterval, uint64ToIntClamp(maxDeliveries))
	srv.jobQueue = jq
	go refreshWorkerRuntimeSettingsDirect(ctx, pgStore, workerState, func(state *workerRuntimeState) {
		if state == nil || jq == nil {
			return
		}
		jq.SetMaxAttempts(uint64ToIntClamp(state.MaxDeliveries()))
	})

	workerCount := configuredJobWorkerCount()
	jqWorker := jobqueue.NewWorker(jq)
	jqWorker.Register(jobqueue.KindTerminalCommand, srv.handleTerminalCommandJob(&counters.processed))
	jqWorker.Register(jobqueue.KindActionRun, srv.handleActionRunJob(&counters.processedActions))
	jqWorker.Register(jobqueue.KindUpdateRun, srv.handleUpdateRunJob(&counters.processedUpdates))
	jqWorker.OnDeadLetter(srv.recordDeadLetter)
	for i := 0; i < workerCount; i++ {
		go jqWorker.Run(ctx)
	}

	return workerBootstrap{
		state:            workerState,
		retentionTracker: retentionTracker,
		counters:         counters,
	}
}

func configuredJobWorkerCount() int {
	raw := strings.TrimSpace(os.Getenv("JOB_WORKERS"))
	if raw == "" {
		return 4
	}
	workerCount, err := strconv.Atoi(raw)
	if err != nil {
		return 4
	}
	if workerCount < 1 {
		return 1
	}
	return workerCount
}
