package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

type workerQueryStatsReader interface {
	TopQueryStats(limit int) ([]persistence.QueryStat, error)
}

func (s *apiServer) workerStatsHandler(
	workerState *workerRuntimeState,
	retentionTracker *retentionState,
	processed, processedActions, processedUpdates *atomic.Uint64,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retentionTracker.Mu.RLock()
		activeRetentionInterval := workerState.RetentionInterval()
		retentionSnapshot := map[string]any{
			"enabled":               true,
			"interval":              activeRetentionInterval.String(),
			"settings":              workerFormatRetentionSettings(retentionTracker.Settings),
			"last_run":              retentionTracker.LastRun,
			"last_error":            retentionTracker.LastErr,
			"last_total_deleted":    retentionTracker.LastRun.TotalDeleted(),
			"last_run_completed_at": retentionTracker.LastRun.RanAt,
		}
		retentionTracker.Mu.RUnlock()
		executorCfg := loadCommandExecutorConfig()

		connectedAgents := 0
		if s.agentMgr != nil {
			connectedAgents = s.agentMgr.Count()
		}
		var queryStatsReader workerQueryStatsReader
		if s.db != nil {
			queryStatsReader = s.db
		}
		queryStatsLimit := parseWorkerQueryStatsLimit(r.URL.Query().Get("query_limit"))

		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"processed_jobs":        processed.Load(),
			"processed_action_runs": processedActions.Load(),
			"processed_update_runs": processedUpdates.Load(),
			"connected_agents":      connectedAgents,
			"queue": map[string]any{
				"max_deliveries": workerState.MaxDeliveries(),
			},
			"terminal_executor": map[string]any{
				"mode":             executorCfg.Mode,
				"timeout":          executorCfg.Timeout.String(),
				"max_output_bytes": executorCfg.MaxOutputBytes,
			},
			"retention":   retentionSnapshot,
			"performance": workerPerformanceSnapshot(queryStatsReader, queryStatsLimit),
		})
	}
}

func parseWorkerQueryStatsLimit(raw string) int {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return 5
	}
	if value == "all" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 5
	}
	if parsed < 0 {
		return 5
	}
	if parsed > 5000 {
		return 5000
	}
	return parsed
}

func workerPerformanceSnapshot(reader workerQueryStatsReader, queryLimit int) map[string]any {
	snapshot := map[string]any{
		"pg_stat_statements_enabled": false,
		"query_limit":                queryLimit,
		"top_queries":                []persistence.QueryStat{},
		"source_queries_top":         sourceQueryDiagnosticsSnapshot(10),
	}
	if reader == nil {
		return snapshot
	}

	topQueries, err := reader.TopQueryStats(queryLimit)
	if err == nil {
		snapshot["pg_stat_statements_enabled"] = true
		snapshot["top_queries"] = topQueries
		return snapshot
	}
	if errors.Is(err, persistence.ErrPGStatStatementsUnavailable) {
		return snapshot
	}
	snapshot["pg_stat_statements_error"] = err.Error()
	return snapshot
}
