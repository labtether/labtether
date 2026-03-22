package groupfeatures

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/updates"
)

// HandleGroupTimeline handles GET /api/v1/groups/:id/timeline.
func (d *Deps) HandleGroupTimeline(w http.ResponseWriter, r *http.Request, groupID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if d.GroupStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
		return
	}

	groupEntry, ok, err := d.GroupStore.GetGroup(groupID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load group")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "group not found")
		return
	}

	window := shared.ParseDurationParam(r.URL.Query().Get("window"), 24*time.Hour, 5*time.Minute, 14*24*time.Hour)
	now := time.Now().UTC()
	to := shared.ParseTimestampParam(r.URL.Query().Get("to"), now)
	from := shared.ParseTimestampParam(r.URL.Query().Get("from"), to.Add(-window))
	limit := shared.ParseLimit(r, 80)
	if limit > 240 {
		limit = 240
	}

	groupAssets, err := d.listAssetsByGroup(groupID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list group assets")
		return
	}
	assetGroup := make(map[string]string, len(groupAssets))
	for _, assetEntry := range groupAssets {
		assetGroup[assetEntry.ID] = groupID
	}
	groupAssetIDs := groupAssetIDsFromAssets(groupAssets)

	logLimit := limit * 4
	if logLimit < 200 {
		logLimit = 200
	}
	if logLimit > 1000 {
		logLimit = 1000
	}

	var (
		logEvents            []logs.Event
		reliabilityLogEvents []logs.Event
		actionRuns           []actions.Run
		updateRuns           []updates.Run
		queryErr             error
		queryErrMu           sync.Mutex
		queryWG              sync.WaitGroup
	)

	runQuery := func(fn func() error) {
		queryWG.Add(1)
		go func() {
			defer queryWG.Done()
			if err := fn(); err != nil {
				queryErrMu.Lock()
				if queryErr == nil {
					queryErr = err
				}
				queryErrMu.Unlock()
			}
		}()
	}

	runQuery(func() error {
		var err error
		logEvents, err = d.LogStore.QueryEvents(logs.QueryRequest{
			From:          from,
			To:            to,
			Limit:         logLimit,
			GroupID:       groupID,
			GroupAssetIDs: groupAssetIDs,
		})
		if err != nil {
			return fmt.Errorf("failed to query logs: %w", err)
		}
		return nil
	})
	runQuery(func() error {
		var err error
		reliabilityLogEvents, err = d.LogStore.QueryEvents(logs.QueryRequest{
			From:          from,
			To:            to,
			Limit:         1000,
			GroupID:       groupID,
			GroupAssetIDs: groupAssetIDs,
			FieldKeys:     []string{"group_id"},
		})
		if err != nil {
			return fmt.Errorf("failed to query logs for reliability: %w", err)
		}
		return nil
	})
	runQuery(func() error {
		var err error
		actionRuns, err = d.ActionStore.ListActionRuns(500, 0, "", "")
		if err != nil {
			return fmt.Errorf("failed to list action runs: %w", err)
		}
		return nil
	})
	runQuery(func() error {
		var err error
		updateRuns, err = d.UpdateStore.ListUpdateRuns(500, "")
		if err != nil {
			return fmt.Errorf("failed to list update runs: %w", err)
		}
		return nil
	})
	queryWG.Wait()
	if queryErr != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, queryErr.Error())
		return
	}
	groupLogEvents := shared.FilterLogEventsByGroup(logEvents, groupID, assetGroup)

	timeline := make([]GroupTimelineEvent, 0, limit+16)
	impact := GroupTimelineImpact{}

	for _, event := range groupLogEvents {
		severity := NormalizeLogSeverity(event.Level)
		timeline = append(timeline, GroupTimelineEvent{
			ID:        event.ID,
			Kind:      "log",
			Severity:  severity,
			Title:     fmt.Sprintf("%s log: %s", strings.ToUpper(severity), event.Source),
			Summary:   strings.TrimSpace(event.Message),
			Source:    event.Source,
			AssetID:   event.AssetID,
			Timestamp: event.Timestamp.UTC(),
		})
		switch severity {
		case "error":
			impact.ErrorEvents++
			if strings.EqualFold(strings.TrimSpace(event.Source), "dead_letter") {
				impact.DeadLetters++
			}
		case "warn":
			impact.WarnEvents++
		default:
			impact.InfoEvents++
		}
	}

	for _, run := range actionRuns {
		if run.UpdatedAt.Before(from) || run.UpdatedAt.After(to) {
			continue
		}
		if !shared.ActionRunMatchesGroup(run, groupID, assetGroup) {
			continue
		}

		severity := SeverityFromRunStatus(run.Status)
		timeline = append(timeline, GroupTimelineEvent{
			ID:        "action_" + run.ID,
			Kind:      "action_run",
			Severity:  severity,
			Title:     fmt.Sprintf("Action %s (%s)", run.ID, run.Status),
			Summary:   summarizeActionRun(run),
			RunID:     run.ID,
			AssetID:   run.Target,
			Timestamp: run.UpdatedAt.UTC(),
		})
		switch severity {
		case "error":
			impact.ErrorEvents++
			impact.FailedActions++
		case "warn":
			impact.WarnEvents++
		default:
			impact.InfoEvents++
		}
	}

	updatePlanIDs := make([]string, 0, len(updateRuns))
	for _, run := range updateRuns {
		planID := strings.TrimSpace(run.PlanID)
		if planID == "" {
			continue
		}
		updatePlanIDs = append(updatePlanIDs, planID)
	}
	plansByID, err := d.loadUpdatePlansByID(updatePlanIDs)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to map update runs to plans")
		return
	}
	for _, run := range updateRuns {
		if run.UpdatedAt.Before(from) || run.UpdatedAt.After(to) {
			continue
		}
		plan, ok := plansByID[strings.TrimSpace(run.PlanID)]
		if !ok || !shared.UpdatePlanTouchesGroup(plan, groupID, assetGroup) {
			continue
		}

		severity := SeverityFromRunStatus(run.Status)
		timeline = append(timeline, GroupTimelineEvent{
			ID:        "update_" + run.ID,
			Kind:      "update_run",
			Severity:  severity,
			Title:     fmt.Sprintf("Update run %s (%s)", run.PlanName, run.Status),
			Summary:   strings.TrimSpace(run.Summary),
			RunID:     run.ID,
			Timestamp: run.UpdatedAt.UTC(),
		})
		switch severity {
		case "error":
			impact.ErrorEvents++
			impact.FailedUpdates++
		case "warn":
			impact.WarnEvents++
		default:
			impact.InfoEvents++
		}
	}

	online := 0
	stale := 0
	offline := 0
	for _, assetEntry := range groupAssets {
		switch GroupAssetFreshness(assetEntry.LastSeenAt, now) {
		case "online":
			online++
		case "stale":
			stale++
		default:
			offline++
		}
	}
	impact.AssetsStale = stale
	impact.AssetsOffline = offline

	freshnessSeverity := "info"
	if offline > 0 {
		freshnessSeverity = "error"
	} else if stale > 0 {
		freshnessSeverity = "warn"
	}
	timeline = append(timeline, GroupTimelineEvent{
		ID:        "freshness_snapshot_" + groupID,
		Kind:      "freshness_snapshot",
		Severity:  freshnessSeverity,
		Title:     "Asset freshness snapshot",
		Summary:   fmt.Sprintf("online=%d stale=%d offline=%d", online, stale, offline),
		Timestamp: now,
	})
	switch freshnessSeverity {
	case "error":
		impact.ErrorEvents++
	case "warn":
		impact.WarnEvents++
	default:
		impact.InfoEvents++
	}

	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Timestamp.After(timeline[j].Timestamp)
	})
	if len(timeline) > limit {
		timeline = timeline[:limit]
	}
	impact.TotalEvents = len(timeline)

	failedActionRuns := make([]actions.Run, 0, len(actionRuns))
	for _, run := range actionRuns {
		if strings.EqualFold(strings.TrimSpace(run.Status), actions.StatusFailed) {
			failedActionRuns = append(failedActionRuns, run)
		}
	}
	failedUpdateRuns := make([]updates.Run, 0, len(updateRuns))
	for _, run := range updateRuns {
		if strings.EqualFold(strings.TrimSpace(run.Status), updates.StatusFailed) {
			failedUpdateRuns = append(failedUpdateRuns, run)
		}
	}

	reliabilityComputation, err := d.BuildGroupReliabilityComputationFromInputs(
		from,
		to,
		now,
		groupAssets,
		reliabilityLogEvents,
		failedActionRuns,
		failedUpdateRuns,
		map[string]struct{}{groupID: {}},
	)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to compute group reliability")
		return
	}

	reliability, err := d.buildGroupReliabilityRecordFromComputation(groupEntry, reliabilityComputation)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to compute group reliability")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"generated_at": time.Now().UTC(),
		"from":         from,
		"to":           to,
		"window":       window.String(),
		"group":        groupEntry,
		"impact":       impact,
		"reliability":  reliability,
		"events":       timeline,
	})
}

func (d *Deps) listAssetsByGroup(groupID string) ([]assets.Asset, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, nil
	}
	if groupAssetStore, ok := d.AssetStore.(persistence.GroupAssetStore); ok {
		return groupAssetStore.ListAssetsByGroup(groupID)
	}

	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return nil, err
	}
	out := make([]assets.Asset, 0, len(assetList))
	for _, assetEntry := range assetList {
		if strings.TrimSpace(assetEntry.GroupID) == groupID {
			out = append(out, assetEntry)
		}
	}
	return out, nil
}

func groupAssetIDsFromAssets(assetList []assets.Asset) []string {
	out := make([]string, 0, len(assetList))
	for _, assetEntry := range assetList {
		assetID := strings.TrimSpace(assetEntry.ID)
		if assetID == "" {
			continue
		}
		out = append(out, assetID)
	}
	sort.Strings(out)
	return out
}

func summarizeActionRun(run actions.Run) string {
	if run.Type == actions.RunTypeCommand {
		command := strings.TrimSpace(run.Command)
		if command != "" {
			return command
		}
	}
	if strings.TrimSpace(run.ConnectorID) != "" || strings.TrimSpace(run.ActionID) != "" {
		return strings.TrimSpace(run.ConnectorID) + ":" + strings.TrimSpace(run.ActionID)
	}
	return strings.TrimSpace(run.Type)
}

// NormalizeLogSeverity normalises a log level string to "error", "warn", or "info".
func NormalizeLogSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "error":
		return "error"
	case "warn", "warning":
		return "warn"
	default:
		return "info"
	}
}

// SeverityFromRunStatus maps a run status string to a timeline severity level.
func SeverityFromRunStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed":
		return "error"
	case "queued", "running":
		return "warn"
	default:
		return "info"
	}
}
