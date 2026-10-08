package persistence

import (
	"github.com/labtether/labtether/internal/logs"
	"sort"
	"strings"
	"time"
)

func (m *MemoryLogStore) ListSourcesSince(limit int, from time.Time) ([]logs.SourceSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	from = from.UTC()

	type sourceStats struct {
		count    int
		lastSeen time.Time
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]sourceStats, 16)
	for _, event := range m.events {
		if event.Timestamp.Before(from) {
			continue
		}
		current := stats[event.Source]
		current.count++
		if event.Timestamp.After(current.lastSeen) {
			current.lastSeen = event.Timestamp
		}
		stats[event.Source] = current
	}

	out := make([]logs.SourceSummary, 0, len(stats))
	for source, stat := range stats {
		out = append(out, logs.SourceSummary{
			Source:     source,
			Count:      stat.count,
			LastSeenAt: stat.lastSeen.UTC(),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryLogStore) ListSources(limit int) ([]logs.SourceSummary, error) {
	if limit <= 0 {
		limit = 50
	}

	type sourceStats struct {
		count    int
		lastSeen time.Time
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]sourceStats, 16)
	for _, event := range m.events {
		current := stats[event.Source]
		current.count++
		if event.Timestamp.After(current.lastSeen) {
			current.lastSeen = event.Timestamp
		}
		stats[event.Source] = current
	}

	out := make([]logs.SourceSummary, 0, len(stats))
	for source, stat := range stats {
		out = append(out, logs.SourceSummary{
			Source:     source,
			Count:      stat.count,
			LastSeenAt: stat.lastSeen,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})

	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryLogStore) QuerySourceSummaries(req logs.SourceSummaryRequest) ([]logs.SourceSummary, error) {
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 1000 {
		req.Limit = 1000
	}
	if req.To.IsZero() {
		req.To = time.Now().UTC()
	}
	if req.From.IsZero() {
		req.From = req.To.Add(-24 * time.Hour)
	}

	groupID := strings.TrimSpace(req.GroupID)
	groupAssetIDs := normalizeLogAssetIDs(req.GroupAssetIDs)
	groupAssetSet := map[string]struct{}{}
	if len(groupAssetIDs) > 0 {
		groupAssetSet = make(map[string]struct{}, len(groupAssetIDs))
		for _, candidate := range groupAssetIDs {
			groupAssetSet[candidate] = struct{}{}
		}
	}

	type aggregate struct {
		count    int
		lastSeen time.Time
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]aggregate, 24)
	for i := len(m.events) - 1; i >= 0; i-- {
		event := m.events[i]
		if event.Timestamp.Before(req.From) || event.Timestamp.After(req.To) {
			continue
		}
		if groupID != "" {
			matchesGroup := false
			if eventAssetID := strings.TrimSpace(event.AssetID); eventAssetID != "" {
				if _, ok := groupAssetSet[eventAssetID]; ok {
					matchesGroup = true
				}
			}
			if !matchesGroup && strings.TrimSpace(event.Fields["group_id"]) == groupID {
				matchesGroup = true
			}
			if !matchesGroup {
				continue
			}
		} else if len(groupAssetSet) > 0 {
			if _, ok := groupAssetSet[strings.TrimSpace(event.AssetID)]; !ok {
				continue
			}
		}

		current := stats[event.Source]
		current.count++
		if event.Timestamp.After(current.lastSeen) {
			current.lastSeen = event.Timestamp
		}
		stats[event.Source] = current
	}

	out := make([]logs.SourceSummary, 0, len(stats))
	for source, stat := range stats {
		out = append(out, logs.SourceSummary{
			Source:     source,
			Count:      stat.count,
			LastSeenAt: stat.lastSeen.UTC(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	if len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

func (m *MemoryLogStore) QueryGroupSeverityCounts(req logs.GroupSeverityCountRequest) ([]logs.GroupSeverityCount, error) {
	from := req.From.UTC()
	to := req.To.UTC()
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}

	groupSet := make(map[string]struct{}, len(req.GroupIDs))
	for _, groupID := range req.GroupIDs {
		groupID = strings.TrimSpace(groupID)
		if groupID == "" {
			continue
		}
		groupSet[groupID] = struct{}{}
	}

	assetGroups := make(map[string]string, len(req.AssetGroups))
	for assetID, groupID := range req.AssetGroups {
		assetID = strings.TrimSpace(assetID)
		groupID = strings.TrimSpace(groupID)
		if assetID == "" || groupID == "" {
			continue
		}
		assetGroups[assetID] = groupID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	countCapacity := len(groupSet)
	if countCapacity < 8 {
		countCapacity = 8
	}
	counts := make(map[string]logs.GroupSeverityCount, countCapacity)
	for i := len(m.events) - 1; i >= 0; i-- {
		event := m.events[i]
		if event.Timestamp.Before(from) || event.Timestamp.After(to) {
			continue
		}

		groupID := strings.TrimSpace(assetGroups[strings.TrimSpace(event.AssetID)])
		if groupID == "" {
			groupID = strings.TrimSpace(event.Fields["group_id"])
		}
		if groupID == "" {
			continue
		}
		if len(groupSet) > 0 {
			if _, ok := groupSet[groupID]; !ok {
				continue
			}
		}

		entry := counts[groupID]
		entry.GroupID = groupID
		switch strings.ToLower(strings.TrimSpace(event.Level)) {
		case "error":
			entry.ErrorCount++
			if strings.EqualFold(strings.TrimSpace(event.Source), "dead_letter") {
				entry.DeadLetterCount++
			}
		case "warn", "warning":
			entry.WarnCount++
		}
		counts[groupID] = entry
	}

	out := make([]logs.GroupSeverityCount, 0, len(counts))
	for _, entry := range counts {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].GroupID < out[j].GroupID
	})
	return out, nil
}
