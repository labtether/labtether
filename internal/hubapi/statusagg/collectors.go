package statusagg

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

const statusTelemetryOverviewCacheTTL = 10 * time.Second

// --- Asset helpers ---

// FilterAssetsByGroup returns assets whose GroupID matches groupFilter.
// If groupFilter is empty all assets are returned.
func FilterAssetsByGroup(assetList []assets.Asset, groupFilter string) []assets.Asset {
	groupFilter = strings.TrimSpace(groupFilter)
	if groupFilter == "" {
		if assetList == nil {
			return []assets.Asset{}
		}
		return assetList
	}
	filtered := make([]assets.Asset, 0, len(assetList))
	for _, assetEntry := range assetList {
		if strings.TrimSpace(assetEntry.GroupID) == groupFilter {
			filtered = append(filtered, assetEntry)
		}
	}
	return filtered
}

// AssetGroupLookup builds a map of assetID -> groupID for assets that have a
// non-empty GroupID.
func AssetGroupLookup(assetList []assets.Asset) map[string]string {
	assetGroup := make(map[string]string, len(assetList))
	for _, assetEntry := range assetList {
		groupID := strings.TrimSpace(assetEntry.GroupID)
		if groupID != "" {
			assetGroup[assetEntry.ID] = groupID
		}
	}
	return assetGroup
}

// CountStaleAssets returns the number of assets that are not "online" at now.
func CountStaleAssets(assetList []assets.Asset, now time.Time) int {
	count := 0
	for _, assetEntry := range assetList {
		if AssetFreshness(assetEntry.LastSeenAt, now) != "online" {
			count++
		}
	}
	return count
}

// AssetFreshness returns "online", "stale", or "offline" based on how recently
// the asset was last seen.
func AssetFreshness(lastSeenAt, now time.Time) string {
	if lastSeenAt.IsZero() {
		return "offline"
	}
	diff := now.Sub(lastSeenAt.UTC())
	if diff < 0 {
		return "offline"
	}
	if diff < 65*time.Second {
		return "online"
	}
	if diff < 5*time.Minute {
		return "stale"
	}
	return "offline"
}

// --- Web service summary ---

func (d *Deps) webServiceSummary(assetsFiltered []assets.Asset) (up int, total int) {
	if d.WebServiceCoordinator == nil {
		return 0, 0
	}

	allowedHosts := make(map[string]struct{}, len(assetsFiltered))
	for _, asset := range assetsFiltered {
		allowedHosts[asset.ID] = struct{}{}
	}
	return d.WebServiceCoordinator.SummaryByHosts(allowedHosts)
}

// --- Telemetry overview ---

func (d *Deps) buildTelemetryOverview(assetList []assets.Asset, now time.Time) []shared.AssetTelemetryOverview {
	out := make([]shared.AssetTelemetryOverview, 0, len(assetList))
	if d.TelemetryStore == nil {
		return out
	}

	if batchStore, ok := d.TelemetryStore.(persistence.TelemetrySnapshotBatchStore); ok {
		if cached, hit := d.telemetryOverviewCacheLookup(assetList, now); hit {
			return cached
		}

		assetIDs := CanonicalAssetIDs(assetList)
		snapshots, err := batchStore.SnapshotMany(assetIDs, now)
		if err == nil {
			for _, assetEntry := range assetList {
				metrics := snapshots[assetEntry.ID]
				out = append(out, shared.AssetTelemetryOverview{
					AssetID:    assetEntry.ID,
					Name:       assetEntry.Name,
					Type:       assetEntry.Type,
					Source:     assetEntry.Source,
					GroupID:    assetEntry.GroupID,
					Status:     assetEntry.Status,
					Platform:   assetEntry.Platform,
					LastSeenAt: assetEntry.LastSeenAt,
					Metrics:    metrics,
				})
			}
			d.telemetryOverviewCacheStore(assetList, now, out)
			return out
		}
		log.Printf("status aggregate: failed to batch query telemetry snapshots: %v", err)
	}

	for _, assetEntry := range assetList {
		metrics, err := d.TelemetryStore.Snapshot(assetEntry.ID, now)
		if err != nil {
			log.Printf("status aggregate: failed to query telemetry for %s: %v", assetEntry.ID, err)
			continue
		}
		out = append(out, shared.AssetTelemetryOverview{
			AssetID:    assetEntry.ID,
			Name:       assetEntry.Name,
			Type:       assetEntry.Type,
			Source:     assetEntry.Source,
			GroupID:    assetEntry.GroupID,
			Status:     assetEntry.Status,
			Platform:   assetEntry.Platform,
			LastSeenAt: assetEntry.LastSeenAt,
			Metrics:    metrics,
		})
	}
	d.telemetryOverviewCacheStore(assetList, now, out)
	return out
}

func (d *Deps) telemetryOverviewCacheLookup(
	assetList []assets.Asset,
	now time.Time,
) ([]shared.AssetTelemetryOverview, bool) {
	fingerprint := CanonicalAssetFingerprint(assetList)
	now = now.UTC()

	d.Cache.TelemetryOverviewCacheMu.RLock()
	entry := d.Cache.TelemetryOverviewCache
	d.Cache.TelemetryOverviewCacheMu.RUnlock()

	if entry.AssetFingerprint != fingerprint || !entry.ExpiresAt.After(now) {
		return nil, false
	}
	return append([]shared.AssetTelemetryOverview(nil), entry.Overview...), true
}

func (d *Deps) telemetryOverviewCacheStore(
	assetList []assets.Asset,
	now time.Time,
	overview []shared.AssetTelemetryOverview,
) {
	now = now.UTC()
	d.Cache.TelemetryOverviewCacheMu.Lock()
	d.Cache.TelemetryOverviewCache = TelemetryOverviewCacheEntry{
		AssetFingerprint: CanonicalAssetFingerprint(assetList),
		ExpiresAt:        now.Add(statusTelemetryOverviewCacheTTL),
		Overview:         append([]shared.AssetTelemetryOverview(nil), overview...),
	}
	d.Cache.TelemetryOverviewCacheMu.Unlock()
}

// --- List helpers ---

func (d *Deps) listAssets() []assets.Asset {
	if d.AssetStore == nil {
		return []assets.Asset{}
	}
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		log.Printf("status aggregate: failed to list assets: %v", err)
		return []assets.Asset{}
	}
	if assetList == nil {
		return []assets.Asset{}
	}
	return assetList
}

func (d *Deps) listGroups() []groups.Group {
	if d.GroupStore == nil {
		return []groups.Group{}
	}
	groupList, err := d.GroupStore.ListGroups()
	if err != nil {
		log.Printf("status aggregate: failed to list groups: %v", err)
		return []groups.Group{}
	}
	if groupList == nil {
		return []groups.Group{}
	}
	return groupList
}

func (d *Deps) listSessions() []terminal.Session {
	if d.TerminalStore == nil {
		return []terminal.Session{}
	}
	sessions, err := d.TerminalStore.ListSessions()
	if err != nil {
		log.Printf("status aggregate: failed to list sessions: %v", err)
		return []terminal.Session{}
	}
	if sessions == nil {
		return []terminal.Session{}
	}
	return sessions
}

func (d *Deps) listRecentCommands(limit int) []terminal.Command {
	if d.TerminalStore == nil {
		return []terminal.Command{}
	}
	commands, err := d.TerminalStore.ListRecentCommands(limit)
	if err != nil {
		log.Printf("status aggregate: failed to list recent commands: %v", err)
		return []terminal.Command{}
	}
	if commands == nil {
		return []terminal.Command{}
	}
	return commands
}

func (d *Deps) listRecentAudit(limit int) []audit.Event {
	if d.AuditStore == nil {
		return []audit.Event{}
	}
	events, err := d.AuditStore.List(limit, 0)
	if err != nil {
		log.Printf("status aggregate: failed to list audit events: %v", err)
		return []audit.Event{}
	}
	if events == nil {
		return []audit.Event{}
	}
	return events
}

func (d *Deps) listConnectors() []connectorsdk.Descriptor {
	if d.ConnectorRegistry == nil {
		return []connectorsdk.Descriptor{}
	}
	connectors := d.ConnectorRegistry.List()
	if connectors == nil {
		return []connectorsdk.Descriptor{}
	}
	return connectors
}

func (d *Deps) listRecentLogs(groupFilter string, assetGroup map[string]string) []logs.Event {
	if d.LogStore == nil {
		return []logs.Event{}
	}

	now := time.Now().UTC()
	groupAssetIDs := shared.GroupAssetIDsForGroup(groupFilter, assetGroup)
	events, err := d.LogStore.QueryEvents(logs.QueryRequest{
		From:          now.Add(-time.Hour),
		To:            now,
		Limit:         200,
		GroupID:       groupFilter,
		GroupAssetIDs: groupAssetIDs,
		ExcludeFields: groupFilter == "",
	})
	if err != nil {
		log.Printf("status aggregate: failed to query recent logs: %v", err)
		return []logs.Event{}
	}

	if groupFilter != "" {
		events = shared.FilterLogEventsByGroup(events, groupFilter, assetGroup)
	}
	events = filterHeartbeatNoise(events)
	if len(events) > 12 {
		events = events[:12]
	}
	if events == nil {
		return []logs.Event{}
	}
	return events
}

func filterHeartbeatNoise(events []logs.Event) []logs.Event {
	filtered := make([]logs.Event, 0, len(events))
	for _, event := range events {
		if isHeartbeatNoiseEvent(event) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func isHeartbeatNoiseEvent(event logs.Event) bool {
	message := strings.ToLower(strings.TrimSpace(event.Message))
	return strings.HasPrefix(message, "heartbeat received (")
}

// ListLogSources is the exported version of listLogSources, used by
// cmd/labtether tests via the bridge forwarding method.
func (d *Deps) ListLogSources(groupFilter string, assetGroup map[string]string, caller string) []logs.SourceSummary {
	return d.listLogSources(groupFilter, assetGroup, caller)
}

func (d *Deps) listLogSources(groupFilter string, assetGroup map[string]string, caller string) []logs.SourceSummary {
	if d.LogStore == nil {
		return []logs.SourceSummary{}
	}

	startedAt := time.Now().UTC()
	normalizedCaller := shared.NormalizeSourceQueryCaller(caller, "status.aggregate")
	const limit = 25
	now := startedAt
	windowStart := floorToMinute(now.Add(-24 * time.Hour))
	mode := "unknown"
	cacheHit := false
	var traceErr error
	var result []logs.SourceSummary
	defer func() {
		shared.LogSourceQueryDiagnostic(
			"status/aggregate",
			normalizedCaller,
			mode,
			strings.TrimSpace(groupFilter) != "",
			cacheHit,
			windowStart,
			now,
			limit,
			len(result),
			startedAt,
			traceErr,
		)
	}()

	if groupFilter == "" {
		if recentLister, ok := d.LogStore.(RecentSourceLister); ok {
			mode = "recent_window"
			sources, hit, err := d.listRecentSourcesCached(recentLister, limit, windowStart)
			if err != nil {
				traceErr = err
				log.Printf("status aggregate: failed to list recent log sources: %v", err)
				return []logs.SourceSummary{}
			}
			cacheHit = hit
			if hit {
				mode = "recent_window_cache"
			}
			result = sources
			return result
		}

		mode = "recent_window_fallback_events"
		events, err := d.LogStore.QueryEvents(logs.QueryRequest{
			From:          windowStart,
			To:            now,
			Limit:         1000,
			ExcludeFields: true,
		})
		if err != nil {
			traceErr = err
			log.Printf("status aggregate: failed to query recent logs for source aggregation: %v", err)
			return []logs.SourceSummary{}
		}

		result = aggregateLogSources(events, limit)
		return result
	}

	mode = "group_filtered_window"
	groupAssetIDs := shared.GroupAssetIDsForGroup(groupFilter, assetGroup)
	events, err := d.LogStore.QueryEvents(logs.QueryRequest{
		From:          windowStart,
		To:            now,
		Limit:         1000,
		GroupID:       groupFilter,
		GroupAssetIDs: groupAssetIDs,
		FieldKeys:     []string{"group_id"},
	})
	if err != nil {
		traceErr = err
		log.Printf("status aggregate: failed to query log sources for group filter: %v", err)
		return []logs.SourceSummary{}
	}

	events = shared.FilterLogEventsByGroup(events, groupFilter, assetGroup)
	result = aggregateLogSources(events, limit)
	return result
}

// StatusListRecentSourcesCached is the exported version of listRecentSourcesCached,
// used by cmd/labtether/log_handlers.go to share the cache between the log
// sources endpoint and the status aggregate endpoint.
func (d *Deps) StatusListRecentSourcesCached(
	recentLister RecentSourceLister,
	limit int,
	windowStart time.Time,
) ([]logs.SourceSummary, bool, error) {
	return d.listRecentSourcesCached(recentLister, limit, windowStart)
}

func (d *Deps) listRecentSourcesCached(
	recentLister RecentSourceLister,
	limit int,
	windowStart time.Time,
) ([]logs.SourceSummary, bool, error) {
	type result struct {
		sources  []logs.SourceSummary
		cacheHit bool
	}

	cacheWatermark := time.Unix(0, 0).UTC()
	hasWatermark := false
	if watermarkReader, ok := d.LogStore.(LogWatermarkReader); ok {
		if watermark, err := watermarkReader.LogEventsWatermark(); err == nil {
			cacheWatermark = watermark.UTC()
			hasWatermark = true
			if cached, hit := d.logSourcesCacheLookup(limit, windowStart, cacheWatermark); hit {
				return cached, true, nil
			}
		}
	}

	key := fmt.Sprintf(
		"log-sources:%d:%d:%t:%d",
		limit,
		windowStart.UTC().Unix(),
		hasWatermark,
		cacheWatermark.UnixNano(),
	)

	computed, err, _ := d.Cache.LogSourcesQueryGroup.Do(key, func() (any, error) {
		if hasWatermark {
			if cached, hit := d.logSourcesCacheLookup(limit, windowStart, cacheWatermark); hit {
				return result{sources: cached, cacheHit: true}, nil
			}
		}

		sources, err := recentLister.ListSourcesSince(limit, windowStart)
		if err != nil {
			return result{}, err
		}
		if sources == nil {
			sources = []logs.SourceSummary{}
		}
		if hasWatermark {
			d.logSourcesCacheStore(limit, windowStart, cacheWatermark, sources)
		}
		return result{
			sources:  append([]logs.SourceSummary(nil), sources...),
			cacheHit: false,
		}, nil
	})
	if err != nil {
		return nil, false, err
	}
	casted, ok := computed.(result)
	if !ok {
		return nil, false, fmt.Errorf("unexpected recent source cache result type %T", computed)
	}
	return append([]logs.SourceSummary(nil), casted.sources...), casted.cacheHit, nil
}

// AggregateLogSources is the exported version for use by cmd/labtether tests.
func AggregateLogSources(events []logs.Event, limit int) []logs.SourceSummary {
	return aggregateLogSources(events, limit)
}

func aggregateLogSources(events []logs.Event, limit int) []logs.SourceSummary {
	type sourceAggregate struct {
		Count    int
		LastSeen time.Time
	}
	aggregates := make(map[string]sourceAggregate, 24)
	for _, event := range events {
		current := aggregates[event.Source]
		current.Count++
		if event.Timestamp.After(current.LastSeen) {
			current.LastSeen = event.Timestamp
		}
		aggregates[event.Source] = current
	}

	sources := make([]logs.SourceSummary, 0, len(aggregates))
	for source, aggregate := range aggregates {
		sources = append(sources, logs.SourceSummary{
			Source:     source,
			Count:      aggregate.Count,
			LastSeenAt: aggregate.LastSeen.UTC(),
		})
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].LastSeenAt.After(sources[j].LastSeenAt)
	})
	if len(sources) > limit {
		sources = sources[:limit]
	}
	return sources
}

func (d *Deps) logSourcesCacheLookup(
	limit int,
	windowStart time.Time,
	watermark time.Time,
) ([]logs.SourceSummary, bool) {
	d.Cache.LogSourcesCacheMu.RLock()
	entry := d.Cache.LogSourcesCache
	d.Cache.LogSourcesCacheMu.RUnlock()

	if entry.Limit != limit ||
		!entry.WindowStart.Equal(windowStart.UTC()) ||
		!entry.Watermark.Equal(watermark.UTC()) {
		return nil, false
	}
	return append([]logs.SourceSummary(nil), entry.Sources...), true
}

func (d *Deps) logSourcesCacheStore(
	limit int,
	windowStart time.Time,
	watermark time.Time,
	sources []logs.SourceSummary,
) {
	d.Cache.LogSourcesCacheMu.Lock()
	d.Cache.LogSourcesCache = LogSourcesCacheEntry{
		Limit:       limit,
		WindowStart: windowStart.UTC(),
		Watermark:   watermark.UTC(),
		Sources:     append([]logs.SourceSummary(nil), sources...),
	}
	d.Cache.LogSourcesCacheMu.Unlock()
}

func (d *Deps) listActionRuns(groupFilter string, assetGroup map[string]string) []actions.Run {
	if d.ActionStore == nil {
		return []actions.Run{}
	}
	runs, err := d.ActionStore.ListActionRuns(12, 0, "", "")
	if err != nil {
		log.Printf("status aggregate: failed to list action runs: %v", err)
		return []actions.Run{}
	}
	if groupFilter == "" {
		if runs == nil {
			return []actions.Run{}
		}
		return runs
	}
	filtered := make([]actions.Run, 0, len(runs))
	for _, run := range runs {
		if shared.ActionRunMatchesGroup(run, groupFilter, assetGroup) {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func (d *Deps) listUpdatePlans(limit int) []updates.Plan {
	if d.UpdateStore == nil {
		return []updates.Plan{}
	}
	plans, err := d.UpdateStore.ListUpdatePlans(limit)
	if err != nil {
		log.Printf("status aggregate: failed to list update plans: %v", err)
		return []updates.Plan{}
	}
	if plans == nil {
		return []updates.Plan{}
	}
	return plans
}

func (d *Deps) listUpdateRuns(groupFilter string, assetGroup map[string]string) []updates.Run {
	if d.UpdateStore == nil {
		return []updates.Run{}
	}
	runs, err := d.UpdateStore.ListUpdateRuns(12, "")
	if err != nil {
		log.Printf("status aggregate: failed to list update runs: %v", err)
		return []updates.Run{}
	}
	if groupFilter == "" {
		if runs == nil {
			return []updates.Run{}
		}
		return runs
	}

	planIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		planID := strings.TrimSpace(run.PlanID)
		if planID == "" {
			continue
		}
		planIDs = append(planIDs, planID)
	}
	plansByID, err := d.loadUpdatePlansByID(planIDs)
	if err != nil {
		log.Printf("status aggregate: failed to bulk-load update plans for group filter: %v", err)
	}

	planGroupCache := make(map[string]bool, len(runs))
	filtered := make([]updates.Run, 0, len(runs))
	for _, run := range runs {
		planID := strings.TrimSpace(run.PlanID)
		if planID == "" {
			continue
		}

		touchesGroup := false
		if plan, ok := plansByID[planID]; ok {
			touchesGroup = shared.UpdatePlanTouchesGroup(plan, groupFilter, assetGroup)
		} else {
			var touchErr error
			touchesGroup, touchErr = d.updateRunTouchesGroup(run.PlanID, groupFilter, assetGroup, planGroupCache)
			if touchErr != nil {
				log.Printf("status aggregate: failed to map update run %s to group: %v", run.ID, touchErr)
				continue
			}
		}

		if touchesGroup {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

// LoadDeadLetters is the exported version of loadDeadLetters, used by
// cmd/labtether tests via the bridge forwarding method.
func (d *Deps) LoadDeadLetters() DeadLetterSnapshot {
	return d.loadDeadLetters()
}

func (d *Deps) loadDeadLetters() DeadLetterSnapshot {
	const (
		statusDeadLetterListLimit   = 20
		statusDeadLetterSampleLimit = 400
	)

	snapshot := DeadLetterSnapshot{
		Events:    []shared.DeadLetterEventResponse{},
		Total:     0,
		Analytics: shared.BuildDeadLetterAnalytics(nil, time.Time{}, time.Time{}, 24*time.Hour),
	}
	snapshot.Analytics = shared.DeadLetterAnalyticsResponse{
		Window:          "24h",
		Bucket:          "1h",
		Total:           0,
		Trend:           []shared.DeadLetterTrendPoint{},
		TopComponents:   []shared.DeadLetterTopEntry{},
		TopSubjects:     []shared.DeadLetterTopEntry{},
		TopErrorClasses: []shared.DeadLetterTopEntry{},
	}

	if d.LogStore == nil {
		return snapshot
	}

	window := 24 * time.Hour
	to := time.Now().UTC().Truncate(time.Minute).Add(time.Minute)
	from := to.Add(-window)

	watermark := time.Time{}
	if watermarkReader, ok := d.LogStore.(LogWatermarkReader); ok {
		if current, err := watermarkReader.LogEventsWatermark(); err == nil {
			watermark = current.UTC()
			if cached, hit := d.deadLetterCacheLookup(from, to, watermark); hit {
				return cached
			}
		}
	}

	deadLetters, err := shared.QueryDeadLetterEventResponses(d.LogStore, from, to, statusDeadLetterSampleLimit)
	if err != nil {
		log.Printf("status aggregate: failed to query dead-letter events: %v", err)
		return snapshot
	}

	if len(deadLetters) > statusDeadLetterListLimit {
		snapshot.Events = deadLetters[:statusDeadLetterListLimit]
	} else {
		snapshot.Events = deadLetters
	}

	total := len(deadLetters)
	if counted, countErr := shared.CountDeadLetterEvents(d.LogStore, from, to); countErr != nil {
		log.Printf("status aggregate: failed to count dead-letter events: %v", countErr)
	} else if counted > total {
		total = counted
	}

	analytics := shared.BuildDeadLetterAnalytics(deadLetters, from, to, window)
	analytics = shared.DeadLetterAnalyticsWithTotal(analytics, total, window)
	if analytics.Trend == nil {
		analytics.Trend = []shared.DeadLetterTrendPoint{}
	}
	if analytics.TopComponents == nil {
		analytics.TopComponents = []shared.DeadLetterTopEntry{}
	}
	if analytics.TopSubjects == nil {
		analytics.TopSubjects = []shared.DeadLetterTopEntry{}
	}
	if analytics.TopErrorClasses == nil {
		analytics.TopErrorClasses = []shared.DeadLetterTopEntry{}
	}

	snapshot.Total = total
	snapshot.Analytics = analytics
	if !watermark.IsZero() {
		d.deadLetterCacheStore(from, to, watermark, snapshot)
	}
	return snapshot
}

func (d *Deps) deadLetterCacheLookup(
	windowStart time.Time,
	windowEnd time.Time,
	watermark time.Time,
) (DeadLetterSnapshot, bool) {
	d.Cache.DeadLetterCacheMu.RLock()
	entry := d.Cache.DeadLetterCache
	d.Cache.DeadLetterCacheMu.RUnlock()

	if !entry.WindowStart.Equal(windowStart.UTC()) ||
		!entry.WindowEnd.Equal(windowEnd.UTC()) ||
		!entry.Watermark.Equal(watermark.UTC()) {
		return DeadLetterSnapshot{}, false
	}
	return cloneDeadLetterSnapshot(entry.Snapshot), true
}

func (d *Deps) deadLetterCacheStore(
	windowStart time.Time,
	windowEnd time.Time,
	watermark time.Time,
	snapshot DeadLetterSnapshot,
) {
	d.Cache.DeadLetterCacheMu.Lock()
	d.Cache.DeadLetterCache = DeadLetterCacheEntry{
		WindowStart: windowStart.UTC(),
		WindowEnd:   windowEnd.UTC(),
		Watermark:   watermark.UTC(),
		Snapshot:    cloneDeadLetterSnapshot(snapshot),
	}
	d.Cache.DeadLetterCacheMu.Unlock()
}

func cloneDeadLetterSnapshot(snapshot DeadLetterSnapshot) DeadLetterSnapshot {
	cloned := DeadLetterSnapshot{
		Events:    append([]shared.DeadLetterEventResponse(nil), snapshot.Events...),
		Total:     snapshot.Total,
		Analytics: snapshot.Analytics,
	}
	cloned.Analytics.Trend = append([]shared.DeadLetterTrendPoint(nil), snapshot.Analytics.Trend...)
	cloned.Analytics.TopComponents = append([]shared.DeadLetterTopEntry(nil), snapshot.Analytics.TopComponents...)
	cloned.Analytics.TopSubjects = append([]shared.DeadLetterTopEntry(nil), snapshot.Analytics.TopSubjects...)
	cloned.Analytics.TopErrorClasses = append([]shared.DeadLetterTopEntry(nil), snapshot.Analytics.TopErrorClasses...)
	return cloned
}

// floorToMinute truncates t to the previous minute boundary.
func floorToMinute(t time.Time) time.Time {
	return t.UTC().Truncate(time.Minute)
}
