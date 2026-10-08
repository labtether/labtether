package groupfeatures

import (
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
	"strings"
	"time"
)

func (d *Deps) loadGroupReliabilityLogCounts(
	from time.Time,
	to time.Time,
	assetGroup map[string]string,
	knownGroupIDs map[string]struct{},
) (map[string]groupReliabilityLogCounts, error) {
	counts := make(map[string]groupReliabilityLogCounts, maxInt(len(knownGroupIDs), 16))
	if d.LogStore == nil {
		return counts, nil
	}

	groupIDs := make([]string, 0, len(knownGroupIDs))
	for groupID := range knownGroupIDs {
		groupID = strings.TrimSpace(groupID)
		if groupID == "" {
			continue
		}
		groupIDs = append(groupIDs, groupID)
	}

	if severityStore, ok := d.LogStore.(persistence.LogGroupSeverityCountStore); ok {
		rows, err := severityStore.QueryGroupSeverityCounts(logs.GroupSeverityCountRequest{
			From:        from,
			To:          to,
			AssetGroups: assetGroup,
			GroupIDs:    groupIDs,
		})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			groupID := strings.TrimSpace(row.GroupID)
			if groupID == "" {
				continue
			}
			if len(knownGroupIDs) > 0 {
				if _, ok := knownGroupIDs[groupID]; !ok {
					continue
				}
			}
			counts[groupID] = groupReliabilityLogCounts{
				Error:      row.ErrorCount,
				Warn:       row.WarnCount,
				DeadLetter: row.DeadLetterCount,
			}
		}
		return counts, nil
	}

	logEvents, err := d.LogStore.QueryEvents(logs.QueryRequest{
		From:      from,
		To:        to,
		Limit:     1000,
		FieldKeys: []string{"group_id"},
	})
	if err != nil {
		return nil, err
	}
	for _, event := range logEvents {
		groupID := strings.TrimSpace(assetGroup[strings.TrimSpace(event.AssetID)])
		if groupID == "" {
			groupID = strings.TrimSpace(event.Fields["group_id"])
		}
		if groupID == "" {
			continue
		}
		if len(knownGroupIDs) > 0 {
			if _, ok := knownGroupIDs[groupID]; !ok {
				continue
			}
		}
		entry := counts[groupID]
		switch NormalizeLogSeverity(event.Level) {
		case "error":
			entry.Error++
			if strings.EqualFold(strings.TrimSpace(event.Source), "dead_letter") {
				entry.DeadLetter++
			}
		case "warn":
			entry.Warn++
		}
		counts[groupID] = entry
	}
	return counts, nil
}

func (d *Deps) loadGroupReliabilityFailedActions(
	from time.Time,
	to time.Time,
	assetGroup map[string]string,
	knownGroupIDs map[string]struct{},
) (map[string]int, error) {
	counts := make(map[string]int, maxInt(len(knownGroupIDs), 16))
	if d.ActionStore == nil {
		return counts, nil
	}

	const pageSize = 500
	offset := 0
	for {
		runs, err := d.ActionStore.ListActionRuns(pageSize, offset, "", actions.StatusFailed)
		if err != nil {
			return nil, err
		}
		if len(runs) == 0 {
			return counts, nil
		}

		for _, run := range runs {
			if run.UpdatedAt.After(to) {
				continue
			}
			if run.UpdatedAt.Before(from) {
				continue
			}
			for _, groupID := range actionRunMatchedGroups(run, assetGroup, knownGroupIDs) {
				counts[groupID]++
			}
		}

		if len(runs) < pageSize || runs[len(runs)-1].UpdatedAt.Before(from) {
			return counts, nil
		}
		offset += len(runs)
	}
}

func (d *Deps) loadGroupReliabilityFailedUpdates(
	from time.Time,
	to time.Time,
	assetGroup map[string]string,
	knownGroupIDs map[string]struct{},
) (map[string]int, error) {
	counts := make(map[string]int, maxInt(len(knownGroupIDs), 16))
	if d.UpdateStore == nil {
		return counts, nil
	}

	const pageSize = 500
	offset := 0
	planTouchedGroups := make(map[string][]string)
	var pageStore persistence.UpdateRunPageStore
	if store, ok := d.UpdateStore.(persistence.UpdateRunPageStore); ok {
		pageStore = store
	}

	for {
		var (
			runs []updates.Run
			err  error
		)
		if pageStore != nil {
			runs, err = pageStore.ListUpdateRunsPage(pageSize, offset, updates.StatusFailed)
		} else {
			if offset > 0 {
				return counts, nil
			}
			runs, err = d.UpdateStore.ListUpdateRuns(pageSize, updates.StatusFailed)
		}
		if err != nil {
			return nil, err
		}
		if len(runs) == 0 {
			return counts, nil
		}

		missingPlanIDs := make([]string, 0, len(runs))
		for _, run := range runs {
			planID := strings.TrimSpace(run.PlanID)
			if planID == "" {
				continue
			}
			if _, ok := planTouchedGroups[planID]; ok {
				continue
			}
			missingPlanIDs = append(missingPlanIDs, planID)
		}
		plansByID, err := d.loadUpdatePlansByID(missingPlanIDs)
		if err != nil {
			return nil, err
		}
		for _, planID := range missingPlanIDs {
			plan, ok := plansByID[planID]
			if !ok {
				planTouchedGroups[planID] = nil
				continue
			}
			planTouchedGroups[planID] = updatePlanTouchedGroups(plan, assetGroup, knownGroupIDs)
		}

		for _, run := range runs {
			if run.UpdatedAt.After(to) {
				continue
			}
			if run.UpdatedAt.Before(from) {
				continue
			}
			for _, groupID := range planTouchedGroups[strings.TrimSpace(run.PlanID)] {
				counts[groupID]++
			}
		}

		if len(runs) < pageSize || runs[len(runs)-1].UpdatedAt.Before(from) {
			return counts, nil
		}
		offset += len(runs)
	}
}

func actionRunMatchedGroups(
	run actions.Run,
	assetGroup map[string]string,
	knownGroupIDs map[string]struct{},
) []string {
	groupIDs := make(map[string]struct{}, 2)
	if target := strings.TrimSpace(run.Target); target != "" {
		if groupID := strings.TrimSpace(assetGroup[target]); groupID != "" {
			groupIDs[groupID] = struct{}{}
		}
	}
	if run.Params != nil {
		if groupID := strings.TrimSpace(run.Params["group_id"]); groupID != "" {
			if len(knownGroupIDs) == 0 {
				groupIDs[groupID] = struct{}{}
			} else if _, ok := knownGroupIDs[groupID]; ok {
				groupIDs[groupID] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(groupIDs))
	for groupID := range groupIDs {
		out = append(out, groupID)
	}
	return out
}

func updatePlanTouchedGroups(
	plan updates.Plan,
	assetGroup map[string]string,
	knownGroupIDs map[string]struct{},
) []string {
	groupIDs := make(map[string]struct{}, len(plan.Targets))
	for _, target := range plan.Targets {
		trimmed := strings.TrimSpace(target)
		if trimmed == "" {
			continue
		}
		if len(knownGroupIDs) > 0 {
			if _, ok := knownGroupIDs[trimmed]; ok {
				groupIDs[trimmed] = struct{}{}
			}
		}
		if groupID := strings.TrimSpace(assetGroup[trimmed]); groupID != "" {
			groupIDs[groupID] = struct{}{}
		}
	}
	out := make([]string, 0, len(groupIDs))
	for groupID := range groupIDs {
		out = append(out, groupID)
	}
	return out
}

// loadUpdatePlansByID loads update plans by their IDs and returns a map keyed
// by plan ID for fast lookup. Mirrors the same method that was on apiServer.
func (d *Deps) loadUpdatePlansByID(planIDs []string) (map[string]updates.Plan, error) {
	out := make(map[string]updates.Plan, len(planIDs))
	if d.UpdateStore == nil || len(planIDs) == 0 {
		return out, nil
	}
	for _, id := range planIDs {
		if _, exists := out[id]; exists {
			continue
		}
		plan, ok, err := d.UpdateStore.GetUpdatePlan(id)
		if err != nil {
			return out, err
		}
		if ok {
			out[id] = plan
		}
	}
	return out, nil
}
