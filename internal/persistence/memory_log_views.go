package persistence

import (
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/logs"
	"sort"
	"strings"
	"time"
)

func (m *MemoryLogStore) SaveView(actorID string, req logs.SavedViewRequest) (logs.SavedView, error) {
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = idgen.New("view")
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	viewKey := ownedLogViewKey(actorID, id)
	if _, ok := m.views[viewKey]; ok {
		return logs.SavedView{}, ErrAlreadyExists
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

	view.CreatedAt = now
	view.UpdatedAt = now

	m.views[viewKey] = view
	return view, nil
}

func (m *MemoryLogStore) ListViews(actorID string, limit int) ([]logs.SavedView, error) {
	if limit <= 0 {
		limit = 50
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]logs.SavedView, 0, len(m.views))
	prefix := actorID + "::"
	for key, view := range m.views {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
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

func (m *MemoryLogStore) GetView(actorID, id string) (logs.SavedView, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	view, ok := m.views[ownedLogViewKey(actorID, id)]
	if !ok {
		return logs.SavedView{}, false, nil
	}
	return view, true, nil
}

func (m *MemoryLogStore) UpdateView(actorID, id string, req logs.SavedViewRequest) (logs.SavedView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	viewKey := ownedLogViewKey(actorID, id)
	view, ok := m.views[viewKey]
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
	m.views[viewKey] = view
	return view, nil
}

func (m *MemoryLogStore) DeleteView(actorID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	viewKey := ownedLogViewKey(actorID, id)
	if _, ok := m.views[viewKey]; !ok {
		return ErrNotFound
	}
	delete(m.views, viewKey)
	return nil
}

func ownedLogViewKey(actorID, viewID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	return actorID + "::" + strings.TrimSpace(viewID)
}
