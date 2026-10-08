package statusagg

import (
	"fmt"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"log"
	"sort"
	"strings"
	"time"
)

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
