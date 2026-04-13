package persistence

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/modelmap"
	"github.com/labtether/labtether/internal/terminal"
)

type MemoryTerminalStore struct {
	store *terminal.Store
}

func NewMemoryTerminalStore() *MemoryTerminalStore {
	return &MemoryTerminalStore{store: terminal.NewStore()}
}

func (m *MemoryTerminalStore) CreateSession(req terminal.CreateSessionRequest) (terminal.Session, error) {
	return m.store.CreateSession(req), nil
}

func (m *MemoryTerminalStore) UpdateSession(session terminal.Session) error {
	return m.store.UpdateSession(session)
}

func (m *MemoryTerminalStore) GetSession(id string) (terminal.Session, bool, error) {
	session, ok := m.store.GetSession(id)
	return session, ok, nil
}

func (m *MemoryTerminalStore) ListSessions() ([]terminal.Session, error) {
	return m.store.ListSessions(), nil
}

func (m *MemoryTerminalStore) DeleteTerminalSession(id string) error {
	return m.store.DeleteTerminalSession(id)
}

func (m *MemoryTerminalStore) AddCommand(sessionID string, req terminal.CreateCommandRequest, target, mode string) (terminal.Command, error) {
	return m.store.AddCommand(sessionID, req, target, mode)
}

func (m *MemoryTerminalStore) UpdateCommandResult(sessionID, commandID, status, output string) error {
	return m.store.UpdateCommandResult(sessionID, commandID, status, output)
}

func (m *MemoryTerminalStore) ListCommands(sessionID string) ([]terminal.Command, error) {
	return m.store.ListCommands(sessionID)
}

func (m *MemoryTerminalStore) ListRecentCommands(limit int) ([]terminal.Command, error) {
	sessions := m.store.ListSessions()
	all := make([]terminal.Command, 0, 64)
	for _, session := range sessions {
		commands, err := m.store.ListCommands(session.ID)
		if err != nil {
			continue
		}
		all = append(all, commands...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}

	out := make([]terminal.Command, len(all))
	copy(out, all)
	return out, nil
}

func (m *MemoryTerminalStore) CreateOrUpdatePersistentSession(req terminal.CreatePersistentSessionRequest) (terminal.PersistentSession, error) {
	return m.store.CreateOrUpdatePersistentSession(req), nil
}

func (m *MemoryTerminalStore) GetPersistentSession(id string) (terminal.PersistentSession, bool, error) {
	persistent, ok := m.store.GetPersistentSession(id)
	return persistent, ok, nil
}

func (m *MemoryTerminalStore) ListPersistentSessions() ([]terminal.PersistentSession, error) {
	return m.store.ListPersistentSessions(), nil
}

func (m *MemoryTerminalStore) ListPersistentSessionsByActor(actorID string) ([]terminal.PersistentSession, error) {
	all := m.store.ListPersistentSessions()
	out := make([]terminal.PersistentSession, 0, len(all))
	for _, persistent := range all {
		if strings.TrimSpace(persistent.ActorID) == strings.TrimSpace(actorID) {
			out = append(out, persistent)
		}
	}
	return out, nil
}

func (m *MemoryTerminalStore) UpdatePersistentSession(id string, req terminal.UpdatePersistentSessionRequest) (terminal.PersistentSession, error) {
	return m.store.UpdatePersistentSession(id, req)
}

func (m *MemoryTerminalStore) MarkPersistentSessionAttached(id string, attachedAt time.Time) (terminal.PersistentSession, error) {
	return m.store.MarkPersistentSessionAttached(id, attachedAt)
}

func (m *MemoryTerminalStore) MarkPersistentSessionDetached(id string, detachedAt time.Time) (terminal.PersistentSession, error) {
	return m.store.MarkPersistentSessionDetached(id, detachedAt)
}

func (m *MemoryTerminalStore) DeletePersistentSession(id string) error {
	return m.store.DeletePersistentSession(id)
}

func (m *MemoryTerminalStore) MarkPersistentSessionArchived(id string, archivedAt time.Time) (terminal.PersistentSession, error) {
	persistent, ok := m.store.GetPersistentSession(id)
	if !ok {
		return terminal.PersistentSession{}, ErrNotFound
	}
	status := "archived"
	_, err := m.store.UpdatePersistentSession(id, terminal.UpdatePersistentSessionRequest{Status: &status})
	if err != nil {
		return terminal.PersistentSession{}, err
	}
	persistent.Status = "archived"
	persistent.ArchivedAt = &archivedAt
	return persistent, nil
}

func (m *MemoryTerminalStore) ListDetachedOlderThan(threshold time.Time) ([]terminal.PersistentSession, error) {
	all := m.store.ListPersistentSessions()
	out := make([]terminal.PersistentSession, 0, len(all))
	for _, p := range all {
		if p.Status == "detached" && !p.Pinned && p.LastDetachedAt != nil && p.LastDetachedAt.Before(threshold) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *MemoryTerminalStore) ListAttachedSessions() ([]terminal.PersistentSession, error) {
	all := m.store.ListPersistentSessions()
	out := make([]terminal.PersistentSession, 0, len(all))
	for _, p := range all {
		if p.Status == "attached" {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *MemoryTerminalStore) MarkAllAttachedAsDetached() error {
	now := time.Now().UTC()
	all := m.store.ListPersistentSessions()
	for _, p := range all {
		if p.Status == "attached" {
			if _, err := m.store.MarkPersistentSessionDetached(p.ID, now); err != nil {
				return err
			}
		}
	}
	return nil
}

type MemoryAuditStore struct {
	store *audit.Store
}

func NewMemoryAuditStore() *MemoryAuditStore {
	return &MemoryAuditStore{store: audit.NewStore()}
}

func (m *MemoryAuditStore) Append(event audit.Event) error {
	m.store.Append(event)
	return nil
}

func (m *MemoryAuditStore) List(limit, offset int) ([]audit.Event, error) {
	events := m.store.List(limit + offset)
	if offset > 0 && offset < len(events) {
		events = events[offset:]
	} else if offset >= len(events) {
		return []audit.Event{}, nil
	}
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

type MemoryAssetStore struct {
	mu     sync.RWMutex
	assets map[string]assets.Asset
}

func NewMemoryAssetStore() *MemoryAssetStore {
	return &MemoryAssetStore{
		assets: make(map[string]assets.Asset),
	}
}

func applyAssetCanonical(asset assets.Asset) assets.Asset {
	asset.Tags = assets.NormalizeTags(asset.Tags)
	asset.ResourceClass, asset.ResourceKind, asset.Attributes = modelmap.DeriveAssetCanonical(asset.Source, asset.Type, asset.Metadata)
	asset.Attributes = cloneAnyMap(asset.Attributes)
	return asset
}

func (m *MemoryAssetStore) UpsertAssetHeartbeat(req assets.HeartbeatRequest) (assets.Asset, error) {
	now := time.Now().UTC()
	status := req.Status
	if status == "" {
		status = "online"
	}
	groupID := strings.TrimSpace(req.GroupID)

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.assets[req.AssetID]
	if !ok {
		existing.CreatedAt = now
	}

	overrideName := strings.TrimSpace(existing.Metadata[assets.MetadataKeyNameOverride])
	metadata := cloneMetadata(req.Metadata)
	if overrideName != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata[assets.MetadataKeyNameOverride] = overrideName
	}

	existing.ID = req.AssetID
	existing.Type = req.Type
	if overrideName != "" {
		existing.Name = overrideName
	} else {
		existing.Name = req.Name
	}
	existing.Source = req.Source
	if !ok || groupID != "" {
		existing.GroupID = groupID
	}
	existing.Platform = req.Platform
	existing.Status = status
	existing.Metadata = metadata
	existing.UpdatedAt = now
	existing.LastSeenAt = now
	existing = applyAssetCanonical(existing)

	m.assets[req.AssetID] = existing
	existing.Tags = cloneStringSlice(existing.Tags)
	existing.Metadata = cloneMetadata(existing.Metadata)
	existing.Attributes = cloneAnyMap(existing.Attributes)
	return existing, nil
}

func (m *MemoryAssetStore) ListAssets() ([]assets.Asset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]assets.Asset, 0, len(m.assets))
	for _, asset := range m.assets {
		asset = applyAssetCanonical(asset)
		asset.Tags = cloneStringSlice(asset.Tags)
		asset.Metadata = cloneMetadata(asset.Metadata)
		asset.Attributes = cloneAnyMap(asset.Attributes)
		out = append(out, asset)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})

	return out, nil
}

func (m *MemoryAssetStore) ListAssetsByGroup(groupID string) ([]assets.Asset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	groupID = strings.TrimSpace(groupID)
	out := make([]assets.Asset, 0, len(m.assets))
	for _, asset := range m.assets {
		if strings.TrimSpace(asset.GroupID) != groupID {
			continue
		}
		asset = applyAssetCanonical(asset)
		asset.Tags = cloneStringSlice(asset.Tags)
		asset.Metadata = cloneMetadata(asset.Metadata)
		asset.Attributes = cloneAnyMap(asset.Attributes)
		out = append(out, asset)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})

	return out, nil
}

func (m *MemoryAssetStore) UpdateAsset(id string, req assets.UpdateRequest) (assets.Asset, error) {
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.assets[id]
	if !ok {
		return assets.Asset{}, ErrNotFound
	}

	if req.Name != nil {
		existing.Name = strings.TrimSpace(*req.Name)
		if existing.Metadata == nil {
			existing.Metadata = map[string]string{}
		}
		existing.Metadata[assets.MetadataKeyNameOverride] = existing.Name
	}
	if req.GroupID != nil {
		existing.GroupID = strings.TrimSpace(*req.GroupID)
	}
	if req.Tags != nil {
		existing.Tags = assets.NormalizeTags(*req.Tags)
	}
	existing.UpdatedAt = now
	existing = applyAssetCanonical(existing)

	m.assets[id] = existing
	existing.Tags = cloneStringSlice(existing.Tags)
	existing.Metadata = cloneMetadata(existing.Metadata)
	existing.Attributes = cloneAnyMap(existing.Attributes)
	return existing, nil
}

// BackdateLastSeenAt directly sets an asset's LastSeenAt (test helper).
func (m *MemoryAssetStore) BackdateLastSeenAt(assetID string, lastSeenAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.assets[assetID]; ok {
		a.LastSeenAt = lastSeenAt
		m.assets[assetID] = a
	}
}

func (m *MemoryAssetStore) GetAsset(id string) (assets.Asset, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	asset, ok := m.assets[id]
	if !ok {
		return assets.Asset{}, false, nil
	}
	asset = applyAssetCanonical(asset)
	asset.Tags = cloneStringSlice(asset.Tags)
	asset.Metadata = cloneMetadata(asset.Metadata)
	asset.Attributes = cloneAnyMap(asset.Attributes)
	return asset, true, nil
}

func (m *MemoryAssetStore) DeleteAsset(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.assets[id]; !ok {
		return ErrNotFound
	}
	delete(m.assets, id)
	return nil
}
