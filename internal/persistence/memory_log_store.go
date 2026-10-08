package persistence

import (
	"github.com/labtether/labtether/internal/logs"
	"strconv"
	"strings"
	"sync"
	"time"
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
	normalized, _, _, err := normalizeLogEventForInsert(event)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.appendEventLocked(normalized)
	return nil
}

func (m *MemoryLogStore) AppendEvents(events []logs.Event) error {
	if len(events) == 0 {
		return nil
	}
	normalized, _, err := normalizeLogEventsForInsert(events)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, event := range normalized {
		m.appendEventLocked(event)
	}
	return nil
}

func (m *MemoryLogStore) appendEventLocked(event logs.Event) {
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
	if req.Limit > 1000 {
		req.Limit = 1000
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

	out := make([]logs.Event, 0)
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

	out := make([]logs.DeadLetterEvent, 0)
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

func (m *MemoryLogStore) LogEventsWatermark() (time.Time, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.latestWatermark.UTC(), nil
}
