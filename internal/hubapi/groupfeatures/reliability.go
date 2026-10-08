package groupfeatures

import (
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/updates"
	"net/http"
	"sort"
	"strings"
	"time"
)

type groupReliabilityAssetCounts struct {
	Total   int
	Online  int
	Stale   int
	Offline int
}

type groupReliabilityLogCounts struct {
	Error      int
	Warn       int
	DeadLetter int
}

type groupReliabilityComputation struct {
	now             time.Time
	assetGroup      map[string]string
	assetCounts     map[string]groupReliabilityAssetCounts
	logCounts       map[string]groupReliabilityLogCounts
	failedActions   map[string]int
	failedUpdates   map[string]int
	knownGroupIDs   map[string]struct{}
	computationFrom time.Time
	computationTo   time.Time
}

// HandleGroupReliabilityCollection handles GET /api/v1/groups/reliability.
func (d *Deps) HandleGroupReliabilityCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if d.GroupStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
		return
	}

	window := shared.ParseDurationParam(r.URL.Query().Get("window"), 24*time.Hour, 5*time.Minute, 14*24*time.Hour)
	to := shared.ParseTimestampParam(r.URL.Query().Get("to"), time.Now().UTC())
	from := shared.ParseTimestampParam(r.URL.Query().Get("from"), to.Add(-window))

	groupList, err := d.GroupStore.ListGroups()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}

	var records []GroupReliabilityRecord
	if shared.HasAssetRestriction(r.Context()) {
		_, assetList, accessible, ok := d.authorizationScope(w, r)
		if !ok {
			return
		}
		filteredGroups := make([]groups.Group, 0, len(accessible))
		for _, groupEntry := range groupList {
			if _, allowed := accessible[strings.TrimSpace(groupEntry.ID)]; allowed {
				filteredGroups = append(filteredGroups, groupEntry)
			}
		}
		groupList = filteredGroups
		assetList = shared.FilterAssetsByAccess(r.Context(), assetList)
		records, err = d.BuildGroupReliabilityRecordsWithAssets(groupList, assetList, from, to)
	} else {
		records, err = d.BuildGroupReliabilityRecords(groupList, from, to)
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to compute group reliability")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"generated_at": time.Now().UTC(),
		"from":         from,
		"to":           to,
		"window":       window.String(),
		"groups":       records,
	})
}

// HandleGroupReliabilityByID handles GET /api/v1/groups/:id/reliability.
func (d *Deps) HandleGroupReliabilityByID(w http.ResponseWriter, r *http.Request, groupID string) {
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
	if !d.requireGroupAccess(w, r, groupID) {
		return
	}

	window := shared.ParseDurationParam(r.URL.Query().Get("window"), 24*time.Hour, 5*time.Minute, 14*24*time.Hour)
	to := shared.ParseTimestampParam(r.URL.Query().Get("to"), time.Now().UTC())
	from := shared.ParseTimestampParam(r.URL.Query().Get("from"), to.Add(-window))

	record, err := d.BuildGroupReliabilityRecord(groupEntry, from, to)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to compute group reliability")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"generated_at": time.Now().UTC(),
		"from":         from,
		"to":           to,
		"window":       window.String(),
		"group":        record,
	})
}

// SortGroupReliabilityRecords sorts records ascending by score, then name.
func SortGroupReliabilityRecords(records []GroupReliabilityRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].Score == records[j].Score {
			return records[i].Group.Name < records[j].Group.Name
		}
		return records[i].Score < records[j].Score
	})
}

// BuildGroupReliabilityRecord computes the reliability record for a single group
// over the given time window. It is exported so the materializer can call it.
func (d *Deps) BuildGroupReliabilityRecord(groupEntry groups.Group, from, to time.Time) (GroupReliabilityRecord, error) {
	records, err := d.BuildGroupReliabilityRecords([]groups.Group{groupEntry}, from, to)
	if err != nil {
		return GroupReliabilityRecord{}, err
	}
	if len(records) == 0 {
		return GroupReliabilityRecord{}, nil
	}
	return records[0], nil
}

// BuildGroupReliabilityRecords computes reliability records for a group slice
// over the given time window.
func (d *Deps) BuildGroupReliabilityRecords(groupList []groups.Group, from, to time.Time) ([]GroupReliabilityRecord, error) {
	if len(groupList) == 0 {
		return []GroupReliabilityRecord{}, nil
	}

	if len(groupList) == 1 {
		groupID := strings.TrimSpace(groupList[0].ID)
		if groupID != "" {
			if groupAssetStore, ok := d.AssetStore.(persistence.GroupAssetStore); ok {
				assetList, err := groupAssetStore.ListAssetsByGroup(groupID)
				if err != nil {
					return nil, err
				}
				return d.BuildGroupReliabilityRecordsWithAssets(groupList, assetList, from, to)
			}
		}
	}

	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return nil, err
	}
	return d.BuildGroupReliabilityRecordsWithAssets(groupList, assetList, from, to)
}

// BuildGroupReliabilityRecordsWithAssets computes reliability records from a
// caller-supplied asset list so hot paths can avoid a second asset scan.
func (d *Deps) BuildGroupReliabilityRecordsWithAssets(
	groupList []groups.Group,
	assetList []assets.Asset,
	from, to time.Time,
) ([]GroupReliabilityRecord, error) {
	if len(groupList) == 0 {
		return []GroupReliabilityRecord{}, nil
	}

	knownGroupIDs := map[string]struct{}{}
	for _, groupEntry := range groupList {
		if groupID := strings.TrimSpace(groupEntry.ID); groupID != "" {
			knownGroupIDs[groupID] = struct{}{}
		}
	}

	computation, err := d.buildGroupReliabilityComputationWithAssets(from, to, assetList, knownGroupIDs)
	if err != nil {
		return nil, err
	}

	records := make([]GroupReliabilityRecord, 0, len(groupList))
	for _, groupEntry := range groupList {
		record, err := d.buildGroupReliabilityRecordFromComputation(groupEntry, computation)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	SortGroupReliabilityRecords(records)
	return records, nil
}

// groupIDSetFromGroups builds a set of non-empty group IDs.
func groupIDSetFromGroups(groupList []groups.Group) map[string]struct{} {
	groupIDs := make(map[string]struct{}, len(groupList))
	for _, groupEntry := range groupList {
		groupID := strings.TrimSpace(groupEntry.ID)
		if groupID == "" {
			continue
		}
		groupIDs[groupID] = struct{}{}
	}
	return groupIDs
}

func (d *Deps) buildGroupReliabilityComputation(
	from time.Time,
	to time.Time,
	knownGroupIDs map[string]struct{},
) (groupReliabilityComputation, error) {
	if len(knownGroupIDs) == 1 {
		for groupID := range knownGroupIDs {
			groupID = strings.TrimSpace(groupID)
			if groupID == "" {
				continue
			}
			if groupAssetStore, ok := d.AssetStore.(persistence.GroupAssetStore); ok {
				assetList, err := groupAssetStore.ListAssetsByGroup(groupID)
				if err != nil {
					return groupReliabilityComputation{}, err
				}
				return d.buildGroupReliabilityComputationWithAssets(from, to, assetList, knownGroupIDs)
			}
		}
	}

	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return groupReliabilityComputation{}, err
	}

	return d.buildGroupReliabilityComputationWithAssets(from, to, assetList, knownGroupIDs)
}

func (d *Deps) buildGroupReliabilityComputationWithAssets(
	from time.Time,
	to time.Time,
	assetList []assets.Asset,
	knownGroupIDs map[string]struct{},
) (groupReliabilityComputation, error) {
	computation := d.newGroupReliabilityComputation(from, to, time.Now().UTC(), assetList, knownGroupIDs)

	logCounts, err := d.loadGroupReliabilityLogCounts(from, to, computation.assetGroup, knownGroupIDs)
	if err != nil {
		return groupReliabilityComputation{}, err
	}
	failedActions, err := d.loadGroupReliabilityFailedActions(from, to, computation.assetGroup, knownGroupIDs)
	if err != nil {
		return groupReliabilityComputation{}, err
	}
	failedUpdates, err := d.loadGroupReliabilityFailedUpdates(from, to, computation.assetGroup, knownGroupIDs)
	if err != nil {
		return groupReliabilityComputation{}, err
	}

	computation.logCounts = logCounts
	computation.failedActions = failedActions
	computation.failedUpdates = failedUpdates
	return computation, nil
}

// BuildGroupReliabilityComputationFromInputs assembles a reliability computation
// from pre-fetched data. Exported so the timeline handler can reuse it directly.
func (d *Deps) BuildGroupReliabilityComputationFromInputs(
	from time.Time,
	to time.Time,
	now time.Time,
	assetList []assets.Asset,
	logEvents []logs.Event,
	failedActionRuns []actions.Run,
	failedUpdateRuns []updates.Run,
	knownGroupIDs map[string]struct{},
) (groupReliabilityComputation, error) {
	computation := d.newGroupReliabilityComputation(from, to, now, assetList, knownGroupIDs)

	for _, event := range logEvents {
		groupID := ""
		if assetID := strings.TrimSpace(event.AssetID); assetID != "" {
			groupID = computation.assetGroup[assetID]
		}
		if groupID == "" {
			groupID = strings.TrimSpace(event.Fields["group_id"])
		}
		if groupID == "" {
			continue
		}
		if len(computation.knownGroupIDs) > 0 {
			if _, ok := computation.knownGroupIDs[groupID]; !ok {
				continue
			}
		}
		counts := computation.logCounts[groupID]
		level := NormalizeLogSeverity(event.Level)
		if level == "error" {
			counts.Error++
			if strings.EqualFold(strings.TrimSpace(event.Source), "dead_letter") {
				counts.DeadLetter++
			}
		}
		if level == "warn" {
			counts.Warn++
		}
		computation.logCounts[groupID] = counts
	}

	for _, run := range failedActionRuns {
		if run.UpdatedAt.Before(from) || run.UpdatedAt.After(to) {
			continue
		}
		for _, groupID := range actionRunMatchedGroups(run, computation.assetGroup, computation.knownGroupIDs) {
			computation.failedActions[groupID]++
		}
	}

	planIDs := make([]string, 0, len(failedUpdateRuns))
	for _, run := range failedUpdateRuns {
		planID := strings.TrimSpace(run.PlanID)
		if planID != "" {
			planIDs = append(planIDs, planID)
		}
	}
	plansByID, err := d.loadUpdatePlansByID(planIDs)
	if err != nil {
		return groupReliabilityComputation{}, err
	}

	planTouchedGroups := make(map[string][]string, len(plansByID))
	for _, run := range failedUpdateRuns {
		if run.UpdatedAt.Before(from) || run.UpdatedAt.After(to) {
			continue
		}

		planID := strings.TrimSpace(run.PlanID)
		if planID == "" {
			continue
		}

		touched, ok := planTouchedGroups[planID]
		if !ok {
			plan, hasPlan := plansByID[planID]
			if !hasPlan {
				planTouchedGroups[planID] = nil
				continue
			}
			touched = updatePlanTouchedGroups(plan, computation.assetGroup, computation.knownGroupIDs)
			planTouchedGroups[planID] = touched
		}
		for _, groupID := range touched {
			computation.failedUpdates[groupID]++
		}
	}

	return computation, nil
}

func (d *Deps) newGroupReliabilityComputation(
	from time.Time,
	to time.Time,
	now time.Time,
	assetList []assets.Asset,
	knownGroupIDs map[string]struct{},
) groupReliabilityComputation {
	computation := groupReliabilityComputation{
		now:             now.UTC(),
		assetGroup:      make(map[string]string, len(assetList)),
		assetCounts:     make(map[string]groupReliabilityAssetCounts, len(assetList)),
		logCounts:       make(map[string]groupReliabilityLogCounts, 16),
		failedActions:   make(map[string]int, 16),
		failedUpdates:   make(map[string]int, 16),
		knownGroupIDs:   knownGroupIDs,
		computationFrom: from,
		computationTo:   to,
	}

	for _, assetEntry := range assetList {
		groupID := strings.TrimSpace(assetEntry.GroupID)
		if groupID == "" {
			continue
		}
		computation.assetGroup[assetEntry.ID] = groupID

		counts := computation.assetCounts[groupID]
		counts.Total++
		switch GroupAssetFreshness(assetEntry.LastSeenAt, computation.now) {
		case "online":
			counts.Online++
		case "stale":
			counts.Stale++
		default:
			counts.Offline++
		}
		computation.assetCounts[groupID] = counts
	}

	return computation
}

func (d *Deps) buildGroupReliabilityRecordFromComputation(
	groupEntry groups.Group,
	computation groupReliabilityComputation,
) (GroupReliabilityRecord, error) {
	record := GroupReliabilityRecord{Group: groupEntry}

	groupID := strings.TrimSpace(groupEntry.ID)
	assetCounts := computation.assetCounts[groupID]
	logCounts := computation.logCounts[groupID]
	record.AssetsTotal = assetCounts.Total
	record.AssetsOnline = assetCounts.Online
	record.AssetsStale = assetCounts.Stale
	record.AssetsOffline = assetCounts.Offline
	record.ErrorLogs = logCounts.Error
	record.WarnLogs = logCounts.Warn
	record.DeadLetters = logCounts.DeadLetter
	record.FailedActions = computation.failedActions[groupID]
	record.FailedUpdates = computation.failedUpdates[groupID]

	guardrails, err := d.GroupGuardrails(groupEntry.ID, computation.computationTo)
	if err != nil {
		return GroupReliabilityRecord{}, err
	}
	record.MaintenanceActive = len(guardrails.ActiveWindows) > 0
	record.SuppressAlerts = guardrails.SuppressAlerts
	record.BlockActions = guardrails.BlockActions
	record.BlockUpdates = guardrails.BlockUpdates

	record.Score = GroupReliabilityScore(record)
	record.Grade = GroupReliabilityGrade(record.Score)
	return record, nil
}

// GroupAssetFreshness classifies an asset's connectivity freshness.
func GroupAssetFreshness(lastSeen time.Time, now time.Time) string {
	diff := now.Sub(lastSeen.UTC())
	if diff < 0 {
		return "offline"
	}
	if diff < GroupOnlineWindow {
		return "online"
	}
	if diff < GroupStaleWindow {
		return "stale"
	}
	return "offline"
}

// GroupReliabilityScore computes the 0-100 reliability score for a record.
func GroupReliabilityScore(record GroupReliabilityRecord) int {
	score := 100
	score -= record.AssetsOffline * 25
	score -= record.AssetsStale * 12
	score -= minInt(record.FailedActions*6, 24)
	score -= minInt(record.FailedUpdates*8, 24)
	score -= minInt(record.ErrorLogs*2, 20)
	score -= minInt(record.DeadLetters*4, 20)
	if score < 0 {
		return 0
	}
	return score
}

// GroupReliabilityGrade maps a score to a letter grade.
func GroupReliabilityGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 75:
		return "B"
	case score >= 60:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "F"
	}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

// assetGroupLookup builds an assetID→groupID map from the asset list.
// This is the same logic as statusAssetGroupLookup in cmd/labtether.
func assetGroupLookup(assetList []assets.Asset) map[string]string {
	assetGroup := make(map[string]string, len(assetList))
	for _, assetEntry := range assetList {
		groupID := strings.TrimSpace(assetEntry.GroupID)
		if groupID != "" {
			assetGroup[assetEntry.ID] = groupID
		}
	}
	return assetGroup
}
