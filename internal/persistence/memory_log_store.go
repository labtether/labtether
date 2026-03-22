package persistence

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/logs"
)

const maxMemoryLogEvents = 10_000

type MemoryLogStore struct {
	mu              sync.RWMutex
	events          []logs.Event
	views           map[string]logs.SavedView
	latestWatermark time.Time
}

func NewMemoryLogStore() *MemoryLogStore {
	return &MemoryLogStore{
		events:          make([]logs.Event, 0, 128),
		views:           make(map[string]logs.SavedView),
		latestWatermark: time.Unix(0, 0).UTC(),
	}
}

func (m *MemoryLogStore) AppendEvent(event logs.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.appendEventLocked(event)
	return nil
}

func (m *MemoryLogStore) AppendEvents(events []logs.Event) error {
	if len(events) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, event := range events {
		m.appendEventLocked(event)
	}
	return nil
}

func (m *MemoryLogStore) appendEventLocked(event logs.Event) {
	if event.ID == "" {
		event.ID = idgen.New("log")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Level == "" {
		event.Level = "info"
	}
	if event.Source == "" {
		event.Source = "labtether"
	}

	event.Fields = cloneMetadata(event.Fields)
	m.events = append(m.events, event)
	if event.Timestamp.After(m.latestWatermark) {
		m.latestWatermark = event.Timestamp.UTC()
	}

	// Evict oldest 20% when over capacity (amortized cost).
	if len(m.events) > maxMemoryLogEvents {
		dropCount := maxMemoryLogEvents / 5
		m.events = append(m.events[:0:0], m.events[dropCount:]...)
	}
}

func (m *MemoryLogStore) QueryEvents(req logs.QueryRequest) ([]logs.Event, error) {
	if req.Limit <= 0 {
		req.Limit = 200
	}
	if req.To.IsZero() {
		req.To = time.Now().UTC()
	}
	if req.From.IsZero() {
		req.From = req.To.Add(-time.Hour)
	}

	search := strings.ToLower(strings.TrimSpace(req.Search))
	level := strings.ToLower(strings.TrimSpace(req.Level))
	source := strings.TrimSpace(req.Source)
	assetID := strings.TrimSpace(req.AssetID)
	groupID := strings.TrimSpace(req.GroupID)
	groupAssetIDs := normalizeLogAssetIDs(req.GroupAssetIDs)
	groupAssetSet := map[string]struct{}{}
	if len(groupAssetIDs) > 0 {
		groupAssetSet = make(map[string]struct{}, len(groupAssetIDs))
		for _, candidate := range groupAssetIDs {
			groupAssetSet[candidate] = struct{}{}
		}
	}
	fieldKeys := normalizeLogFieldKeys(req.FieldKeys)

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]logs.Event, 0, req.Limit)
	for i := len(m.events) - 1; i >= 0; i-- {
		event := m.events[i]
		if event.Timestamp.Before(req.From) || event.Timestamp.After(req.To) {
			continue
		}
		if assetID != "" && event.AssetID != assetID {
			continue
		}
		if source != "" && event.Source != source {
			continue
		}
		if level != "" && strings.ToLower(event.Level) != level {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(event.Message), search) {
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

		if req.ExcludeFields {
			event.Fields = nil
		} else if len(fieldKeys) > 0 {
			event.Fields = projectLogFields(event.Fields, fieldKeys)
		} else {
			event.Fields = cloneMetadata(event.Fields)
		}
		out = append(out, event)
		if len(out) >= req.Limit {
			break
		}
	}

	return out, nil
}

func (m *MemoryLogStore) QueryDeadLetterEvents(from, to time.Time, limit int) ([]logs.DeadLetterEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]logs.DeadLetterEvent, 0, limit)
	for i := len(m.events) - 1; i >= 0; i-- {
		event := m.events[i]
		if event.Timestamp.Before(from) || event.Timestamp.After(to) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(event.Source), "dead_letter") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(event.Level), "error") {
			continue
		}

		fields := event.Fields
		deliveries, _ := strconv.ParseUint(strings.TrimSpace(fields["deliveries"]), 10, 64)

		id := strings.TrimSpace(fields["event_id"])
		if id == "" {
			id = event.ID
		}

		errorMessage := strings.TrimSpace(fields["error"])
		if errorMessage == "" {
			errorMessage = strings.TrimSpace(event.Message)
		}

		out = append(out, logs.DeadLetterEvent{
			ID:         id,
			Component:  strings.TrimSpace(fields["component"]),
			Subject:    strings.TrimSpace(fields["subject"]),
			Deliveries: deliveries,
			Error:      errorMessage,
			PayloadB64: strings.TrimSpace(fields["payload_b64"]),
			CreatedAt:  event.Timestamp.UTC(),
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *MemoryLogStore) CountDeadLetterEvents(from, to time.Time) (int, error) {
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	total := 0
	for i := len(m.events) - 1; i >= 0; i-- {
		event := m.events[i]
		if event.Timestamp.Before(from) || event.Timestamp.After(to) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(event.Source), "dead_letter") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(event.Level), "error") {
			continue
		}
		total++
	}
	return total, nil
}

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

func (m *MemoryLogStore) LogEventsWatermark() (time.Time, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.latestWatermark.UTC(), nil
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

func (m *MemoryLogStore) SaveView(req logs.SavedViewRequest) (logs.SavedView, error) {
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = idgen.New("view")
	}

	view := logs.SavedView{
		ID:      id,
		Name:    strings.TrimSpace(req.Name),
		AssetID: strings.TrimSpace(req.AssetID),
		Source:  strings.TrimSpace(req.Source),
		Level:   strings.TrimSpace(req.Level),
		Search:  strings.TrimSpace(req.Search),
		Window:  strings.TrimSpace(req.Window),
	}

	if existing, ok := m.views[id]; ok {
		view.CreatedAt = existing.CreatedAt
	} else {
		view.CreatedAt = now
	}
	view.UpdatedAt = now

	m.views[id] = view
	return view, nil
}

func (m *MemoryLogStore) ListViews(limit int) ([]logs.SavedView, error) {
	if limit <= 0 {
		limit = 50
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]logs.SavedView, 0, len(m.views))
	for _, view := range m.views {
		out = append(out, view)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})

	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryLogStore) GetView(id string) (logs.SavedView, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	view, ok := m.views[strings.TrimSpace(id)]
	if !ok {
		return logs.SavedView{}, false, nil
	}
	return view, true, nil
}

func (m *MemoryLogStore) UpdateView(id string, req logs.SavedViewRequest) (logs.SavedView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id = strings.TrimSpace(id)
	view, ok := m.views[id]
	if !ok {
		return logs.SavedView{}, ErrNotFound
	}
	view.Name = strings.TrimSpace(req.Name)
	view.AssetID = strings.TrimSpace(req.AssetID)
	view.Source = strings.TrimSpace(req.Source)
	view.Level = strings.TrimSpace(req.Level)
	view.Search = strings.TrimSpace(req.Search)
	view.Window = strings.TrimSpace(req.Window)
	view.UpdatedAt = time.Now().UTC()
	m.views[id] = view
	return view, nil
}

func (m *MemoryLogStore) DeleteView(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id = strings.TrimSpace(id)
	if _, ok := m.views[id]; !ok {
		return ErrNotFound
	}
	delete(m.views, id)
	return nil
}
