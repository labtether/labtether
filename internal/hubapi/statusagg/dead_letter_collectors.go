package statusagg

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"log"
	"time"
)

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
